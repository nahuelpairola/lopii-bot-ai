package messaging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/subcategory"
)

// needsFirstAccount is true when a non-transfer row has no account_id and the
// user has no default in that row's currency (zero-account first run) — the
// lazy-create trigger for stepCreateFirstAccount.
func needsFirstAccount(seed conversation.Data, has func(cur currency.Currency) bool) bool {
	for _, r := range decodeMovementRows(seed) {
		if movement.TypeFromString(r.Type) == movement.Transfer {
			continue
		}
		if r.AccountID == "" && !has(currency.Currency(r.Currency)) {
			return true
		}
	}
	return false
}

// createErrorCopy maps a guard rejection to specific user copy, falling back
// to the generic error. Mirrors finishAccountCreateFlow's ErrAccountAlreadyExists.
func createErrorCopy(err error) string {
	switch {
	case errors.Is(err, errZeroAmount):
		return msgAmountUnclear
	case errors.Is(err, errCurrencyAccountMismatch):
		return msgCurrencyMismatch
	case errors.Is(err, errNoAccountForCurrency):
		return msgNoAccountCurrency
	case errors.Is(err, errTransferLeg):
		return msgMovementMalformed
	default:
		return msgGenericFlowError
	}
}

// guardReason maps a guard rejection to a stable log value. Mirror of
// createErrorCopy, which maps the same sentinels to user-facing copy.
func guardReason(err error) string {
	switch {
	case errors.Is(err, errZeroAmount):
		return "zero_amount"
	case errors.Is(err, errCurrencyAccountMismatch):
		return "currency_account_mismatch"
	case errors.Is(err, errNoAccountForCurrency):
		return "no_account_for_currency"
	case errors.Is(err, errTransferLeg):
		return "malformed_transfer"
	default:
		return "other"
	}
}

// sendText is a small helper that guards every b.SendMessage call with a
// nil check — b is nil in unit tests that exercise these entry points
// directly (see free_text_test.go), matching the same guard pattern
// already used throughout movement_update_flow.go/movement_delete_flow.go.
func (c *controller) sendText(ctx context.Context, b *bot.Bot, chatID int64, text string) {
	if b == nil {
		return
	}
	b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text})
}

// handleFreeText is the entry point for any message with no flow
// already in progress: Call 1 (router) decides which of the four
// intents it is, and every other function in this file handles one.
func (c *controller) handleFreeText(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) error {
	result, err := c.orchestrator.ClassifyIntent(ctx, text)
	if err != nil {
		slog.ErrorContext(ctx, "intent classification failed", "user_id", userID, "err", err)
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return fmt.Errorf("classify intent: %w", err)
	}

	slog.InfoContext(ctx, "intent classified",
		"user_id", userID,
		"intent", string(result.Intent),
		"needs_confirmation", result.NeedsConfirmation,
	)

	c.logIntent(ctx, userID, text, result.Intent, result.NeedsConfirmation)

	switch result.Intent {
	case orchestrator.IntentQuery:
		answered, qErr := c.handleQuery(ctx, b, chatID, userID, text)
		if answered {
			c.resolveMetric(ctx, userID, outcomeQueryAnswered)
		} else {
			if qErr != nil {
				slog.ErrorContext(ctx, "query failed", "user_id", userID, "err", qErr)
			}
			c.resolveMetric(ctx, userID, outcomeQueryFailed)
		}
		return qErr
	case orchestrator.IntentCreate:
		return c.startMovementCreate(ctx, b, chatID, userID, text, result.NeedsConfirmation)
	case orchestrator.IntentUpdate:
		return c.startMovementUpdate(ctx, b, chatID, userID, text)
	case orchestrator.IntentDelete:
		return c.startMovementDelete(ctx, b, chatID, userID, text)
	case orchestrator.IntentAccountManage:
		return c.startAccountManage(ctx, b, chatID, userID, text)
	case orchestrator.IntentCreateCategory:
		return c.startSubcategorySetup(ctx, b, chatID, userID, text)
	case orchestrator.IntentCategoryManage:
		return c.startCategoryManage(ctx, b, chatID, userID)
	case orchestrator.IntentReminderSet:
		return c.startReminderSetup(ctx, b, chatID, userID)
	case orchestrator.IntentHelp:
		c.sendText(ctx, b, chatID, msgHelp)
		return nil
	default:
		c.sendText(ctx, b, chatID, msgGenericFlowError)
	}
	return nil
}

// startSubcategoryWizard starts the classic 7-step wizard fresh — the
// fallback whenever the LLM path can't produce a trustworthy match/proposal.
func (c *controller) startSubcategoryWizard(ctx context.Context, b *bot.Bot, chatID int64, userID uint64) error {
	prompt, err := c.engine.Start(userID, subcategorySetupFlowName)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return fmt.Errorf("start subcategory_setup flow: %w", err)
	}
	if b != nil {
		c.sendPrompt(ctx, b, chatID, prompt)
	}
	return nil
}

// startSubcategorySetup resolves a CREATE_CATEGORY message with the LLM
// first: an existing-entry match offers reuse (the "regalos ya existía"
// case), a full proposal collapses the 7-step wizard into one confirmation.
// Any doubt → the classic wizard, never a dead end.
func (c *controller) startSubcategorySetup(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) error {
	slog.InfoContext(ctx, "flow started", "flow", subcategorySetupFlowName, "user_id", userID)
	subs, err := c.subcategories.FindAllForUser(userID)
	if err != nil {
		return c.startSubcategoryWizard(ctx, b, chatID, userID)
	}
	taxonomy := make([]orchestrator.TaxonomyEntry, 0, len(subs))
	for _, s := range subs {
		if subcategory.IsReserved(s.Category) {
			continue
		}
		taxonomy = append(taxonomy, orchestrator.TaxonomyEntry{Category: s.Category, Subcategory: s.Subcategory, Description: s.Description})
	}

	res, err := c.orchestrator.ClassifyCategoryCreate(ctx, text, taxonomy)
	if err != nil {
		return c.startSubcategoryWizard(ctx, b, chatID, userID)
	}

	if res.Match != nil {
		existing, err := c.subcategories.FindByCategoryAndSubcategory(userID, res.Match.Category, res.Match.Subcategory)
		if err != nil { // hallucinated match → can't offer it
			return c.startSubcategoryWizard(ctx, b, chatID, userID)
		}
		return c.startCategoryMatchOffer(ctx, b, chatID, userID, existing)
	}

	if res.Proposal == nil { // neither match nor proposal usable → never a dead end
		return c.startSubcategoryWizard(ctx, b, chatID, userID)
	}
	p := res.Proposal
	p.Category, p.Subcategory = strings.TrimSpace(p.Category), strings.TrimSpace(p.Subcategory)
	if p.Category == "" || p.Subcategory == "" || subcategory.IsReserved(p.Category) || subcategory.IsReserved(p.Subcategory) {
		return c.startSubcategoryWizard(ctx, b, chatID, userID)
	}
	if existing, err := c.subcategories.FindByCategoryAndSubcategory(userID, p.Category, p.Subcategory); err == nil {
		return c.startCategoryMatchOffer(ctx, b, chatID, userID, existing) // exact duplicate → offer, don't re-create
	}

	isNew := "true"
	if cats, err := c.subcategories.DistinctCategoriesForUser(userID); err == nil {
		for _, cat := range cats {
			if cat == p.Category {
				isNew = "false"
				break
			}
		}
	}
	icon := strings.TrimSpace(p.Icon)
	if !subcategory.ValidIcon(icon) {
		icon = "" // insertNewSubcategory falls back to IconForCategory / 📂
	}
	seed := conversation.Data{
		keyCategory:               p.Category,
		keyCategoryIsNew:          isNew,
		keyCategoryIcon:           icon,
		keySubcategory:            p.Subcategory,
		keySubcategoryDescription: strings.TrimSpace(p.Description),
	}
	prompt, err := c.engine.StartWithData(userID, categoryProposalConfirmFlowName, seed)
	if err != nil {
		return c.startSubcategoryWizard(ctx, b, chatID, userID)
	}
	if b != nil {
		c.sendPrompt(ctx, b, chatID, prompt)
	}
	return nil
}

// startCategoryManage arranca el flujo de sacar una categoría propia. Antes de
// nada verifica que el usuario tenga alguna: sin eso el picker mostraría solo
// "Cancelar", que es un callejón sin salida disfrazado de flujo.
func (c *controller) startCategoryManage(ctx context.Context, b *bot.Bot, chatID int64, userID uint64) error {
	slog.InfoContext(ctx, "flow started", "flow", categoryManagePickFlowName, "user_id", userID)

	owned, err := c.subcategories.FindOwnedByUser(userID)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return fmt.Errorf("category manage: find owned: %w", err)
	}
	if len(owned) == 0 {
		// Se resuelve la métrica: el bot entendió y respondió bien. Sin esto el
		// evento queda pendiente y el sweeper lo marca "abandoned", que en las
		// métricas de asertividad se lee como una falla del bot.
		c.resolveMetric(ctx, userID, outcomeCategoryManageNoOwn)
		c.sendText(ctx, b, chatID, msgCategoryManageNoOwn)
		return nil
	}

	prompt, err := c.engine.Start(userID, categoryManagePickFlowName)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return fmt.Errorf("start category_manage_pick flow: %w", err)
	}
	if b != nil {
		c.sendPrompt(ctx, b, chatID, prompt)
	}
	return nil
}

// startCategoryMatchOffer seeds and starts category_match_offer from an
// existing taxonomy row.
func (c *controller) startCategoryMatchOffer(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, s *subcategory.Subcategory) error {
	seed := conversation.Data{
		keyCategory:               s.Category,
		keySubcategory:            s.Subcategory,
		keySubcategoryDescription: s.Description,
		keyCategoryIcon:           s.Icon,
	}
	prompt, err := c.engine.StartWithData(userID, categoryMatchOfferFlowName, seed)
	if err != nil {
		return c.startSubcategoryWizard(ctx, b, chatID, userID)
	}
	if b != nil {
		c.sendPrompt(ctx, b, chatID, prompt)
	}
	return nil
}

// startAccountCreate reuses ClassifyOnboarding to prefill the flow when the
// triggering message already states the account name and/or opening balance
// (e.g. "Nueva cuenta: Cedears tengo 1041265"). It seeds only when exactly one
// account is extracted; 0, >1, or an extractor error fall back to a blank flow
// (StartWithData with an empty seed == Start). Currency is never seeded — it
// stays the flow's currency ChoiceStep. No step is auto-skipped: the user still
// confirms every value.
// startAccountManage runs Call 2 account-match and branches: matched →
// manage menu; wants-new → the existing (prefill-seeded) create flow;
// unclear → the candidate picker. Candidates are always seeded — the pick
// step needs them, the menu path skips it via SkipIf.
func (c *controller) startAccountManage(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) error {
	slog.InfoContext(ctx, "flow started", "flow", accountManageFlowName, "user_id", userID)
	accs, err := c.accounts.FindByUserID(userID)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return fmt.Errorf("account manage: find accounts: %w", err)
	}
	if len(accs) == 0 {
		c.resolveMetric(ctx, userID, outcomeAccountCreateRouted)
		return c.startAccountCreate(ctx, b, chatID, userID, text)
	}

	opts := make([]orchestrator.AccountOption, 0, len(accs))
	for _, a := range accs {
		opts = append(opts, orchestrator.AccountOption{ID: uint64(a.ID), Name: a.Name, Currency: a.Currency.String()})
	}
	res, err := c.orchestrator.ResolveAccountManage(ctx, text, opts)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return fmt.Errorf("account manage: resolve: %w", err)
	}
	if res.WantsNewAccount {
		c.resolveMetric(ctx, userID, outcomeAccountCreateRouted)
		return c.startAccountCreate(ctx, b, chatID, userID, text)
	}

	ids := make([]string, 0, len(accs))
	labels := make([]string, 0, len(accs))
	names := make([]string, 0, len(accs))
	curs := make([]string, 0, len(accs))
	for _, a := range accs {
		ids = append(ids, strconv.FormatUint(uint64(a.ID), 10))
		labels = append(labels, a.Name+" ("+a.Currency.String()+")")
		names = append(names, a.Name)
		curs = append(curs, a.Currency.String())
	}
	seed := conversation.Data{
		keyMessage:             text,
		keyCandidateIDs:        encodeStringSlice(ids),
		keyCandidateLabels:     encodeStringSlice(labels),
		keyCandidateNames:      encodeStringSlice(names),
		keyCandidateCurrencies: encodeStringSlice(curs),
	}
	if res.MatchedAccountID != nil {
		// never trust an LLM id blindly — it must exist in the user's list
		for _, a := range accs {
			if uint64(a.ID) == *res.MatchedAccountID {
				seed[keyAccountID] = strconv.FormatUint(uint64(a.ID), 10)
				seed[keyAccountName] = a.Name
				seed[keyAccountCurrency] = a.Currency.String()
				break
			}
		}
	}

	prompt, err := c.engine.StartWithData(userID, accountManageFlowName, seed)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return fmt.Errorf("account manage: start account_manage flow: %w", err)
	}
	if b != nil {
		c.sendPrompt(ctx, b, chatID, prompt)
	}
	return nil
}

func (c *controller) startAccountCreate(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) error {
	slog.InfoContext(ctx, "flow started", "flow", accountCreateFlowName, "user_id", userID)
	seed := c.accountCreateSeed(ctx, text)
	prompt, err := c.engine.StartWithData(userID, accountCreateFlowName, seed)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return fmt.Errorf("start account_create flow: %w", err)
	}
	if b != nil {
		c.sendPrompt(ctx, b, chatID, prompt)
	}
	return nil
}

// accountCreateSeed extracts a prefill seed from the account-create message.
// Returns an empty (non-nil) Data when nothing should be prefilled.
func (c *controller) accountCreateSeed(ctx context.Context, text string) conversation.Data {
	seed := conversation.Data{}
	res, err := c.orchestrator.ClassifyOnboarding(ctx, text)
	if err != nil || len(res.Accounts) != 1 {
		return seed
	}
	d := res.Accounts[0]
	if d.Name != "" {
		seed[keyAccountName] = d.Name
	}
	if amt, err := parseARAmount(d.Balance); err == nil && !amt.IsNegative() {
		seed[keyAccountBalance] = d.Balance
	}
	return seed
}

// startMovementCreate trusts the router's needsConfirmation verdict:
// true routes straight to the confirm gate instead of running Call 2
// CREATE. Only when false does it run Call 2 CREATE and either insert
// directly (no gaps — the frictionless default) or start
// movement_create seeded with whatever was resolved, landing on the
// first real gap.
func (c *controller) startMovementCreate(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string, needsConfirmation bool) error {
	if needsConfirmation {
		c.startMovementConfirm(ctx, b, chatID, userID)
		return nil
	}
	slog.InfoContext(ctx, "flow started", "flow", movementCreateFlowName, "user_id", userID)

	subs, err := c.subcategories.FindAllForUser(userID)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return fmt.Errorf("create: find subcategories: %w", err)
	}
	taxonomy := make([]orchestrator.TaxonomyEntry, 0, len(subs))
	for _, s := range subs {
		taxonomy = append(taxonomy, orchestrator.TaxonomyEntry{Category: s.Category, Subcategory: s.Subcategory, Description: s.Description})
	}

	accs, err := c.accounts.FindByUserID(userID)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return fmt.Errorf("create: find accounts: %w", err)
	}
	accountOptions := make([]orchestrator.AccountOption, 0, len(accs))
	for _, a := range accs {
		accountOptions = append(accountOptions, orchestrator.AccountOption{ID: uint64(a.ID), Name: a.Name, Currency: a.Currency.String()})
	}

	result, err := c.orchestrator.ClassifyCreate(ctx, text, taxonomy, accountOptions, time.Now().Format("2006-01-02"))
	if err != nil {
		// "no había nada que extraer" no es una falla del sistema: el mensaje
		// no traía el dato (típicamente el monto, o era una referencia como
		// "ponelo ahí"). Pedirle lo que falta es la respuesta honesta; mostrar
		// el error genérico deja al usuario sin saber qué hacer.
		if errors.Is(err, orchestrator.ErrNothingToExtract) {
			c.resolveMetric(ctx, userID, outcomeCreateRewrite)
			c.sendText(ctx, b, chatID, msgAskRewrite)
			return nil
		}
		c.resolveMetric(ctx, userID, outcomeCreateFailed)
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return fmt.Errorf("create: classify: %w", err)
	}
	slog.DebugContext(ctx, "create classification result", "result", result)

	seed := buildCreateSeed(result)
	slog.InfoContext(ctx, "create seed built",
		"user_id", userID,
		"movements", len(result.Movements),
		"category_gaps", len(decodeStringSlice(seed, keyPendingCategoryGaps)),
		"account_gaps", len(decodeStringSlice(seed, keyPendingAccountGaps)),
	)
	hasGaps := len(decodeStringSlice(seed, keyPendingCategoryGaps)) > 0 || len(decodeStringSlice(seed, keyPendingAccountGaps)) > 0
	hasFirst := needsFirstAccount(seed, func(cur currency.Currency) bool {
		return c.accounts.HasDefaultForCurrency(userID, cur)
	})

	if !hasGaps && !hasFirst {
		seed[conversation.UserIDKey] = userID
		inserted, err := c.resolveAndInsertMovements(seed)
		if err != nil {
			var short *insufficientFunds
			if errors.As(err, &short) {
				gateSeed := copyData(seed)
				gateSeed["_gate_prompt"] = msgInsufficientFunds(short.shortfalls)
				prompt, serr := c.engine.StartWithData(userID, movementNegativeConfirmFlowName, gateSeed)
				if serr != nil {
					c.sendText(ctx, b, chatID, msgGenericFlowError)
					return fmt.Errorf("create: start negative-confirm flow: %w", serr)
				}
				if b != nil {
					c.sendPrompt(ctx, b, chatID, prompt)
				}
				return nil
			}
			c.resolveMetric(ctx, userID, outcomeCreateFailed)
			c.sendText(ctx, b, chatID, createErrorCopy(err))
			return fmt.Errorf("create insert (%s): %w", guardReason(err), err)
		}
		c.resolveMetric(ctx, userID, outcomeCreateInserted)
		c.sendText(ctx, b, chatID, msgConfirmMovements(inserted))
		return nil
	}

	prompt, err := c.engine.StartWithData(userID, movementCreateFlowName, seed)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return fmt.Errorf("create: start movement_create flow: %w", err)
	}
	if b != nil {
		c.sendPrompt(ctx, b, chatID, prompt)
	}
	return nil
}

// startMovementUpdate resolves which existing movement(s) the message
// refers to via resolveCandidates (pg_trgm search, default 7-day
// window) and branches on how many candidates come back.
func (c *controller) startMovementUpdate(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) error {
	slog.InfoContext(ctx, "flow started", "flow", movementUpdatePickFlowName, "user_id", userID)
	candidates, err := c.resolveCandidates(userID, text, "", "")
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return fmt.Errorf("update: resolve candidates: %w", err)
	}

	switch len(candidates) {
	case 0:
		slog.InfoContext(ctx, "no candidates found", "user_id", userID)
		c.resolveMetric(ctx, userID, outcomeNoCandidates)
		c.sendText(ctx, b, chatID, msgNoCandidatesFound)
	case 1:
		rows := make([]movementRow, 0, len(candidates[0].Movements))
		oldIDs := make([]string, 0, len(candidates[0].Movements))
		for _, m := range candidates[0].Movements {
			rows = append(rows, movementToRow(m))
			oldIDs = append(oldIDs, strconv.FormatUint(uint64(m.ID), 10))
		}
		if err := c.proceedToUpdateConfirm(ctx, b, chatID, userID, text, candidates[0].TransactionID, oldIDs, rows); err != nil {
			c.sendText(ctx, b, chatID, msgGenericFlowError)
			return fmt.Errorf("update: proceed to confirm: %w", err)
		}
	default:
		labels := make([]string, 0, len(candidates))
		for _, g := range candidates {
			labels = append(labels, candidateLabel(g))
		}
		seed := conversation.Data{
			keyMessage:         text,
			keyCandidateLabels: encodeStringSlice(labels),
			keyCandidateGroups: encodeCandidateGroups(candidates),
		}
		prompt, err := c.engine.StartWithData(userID, movementUpdatePickFlowName, seed)
		if err != nil {
			c.sendText(ctx, b, chatID, msgGenericFlowError)
			return fmt.Errorf("update: start movement_update_pick flow: %w", err)
		}
		if b != nil {
			c.sendPrompt(ctx, b, chatID, prompt)
		}
	}
	return nil
}

// startMovementDelete mirrors startMovementUpdate's reference
// resolution — since deleting needs no second LLM call once a candidate
// is known (see movement_delete_flow.go), it seeds movement_delete
// directly with resolved_index already set whenever there's exactly one
// candidate, letting the flow's Skip mechanism bypass the picker
// entirely.
func (c *controller) startMovementDelete(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) error {
	slog.InfoContext(ctx, "flow started", "flow", movementDeleteFlowName, "user_id", userID)
	candidates, err := c.resolveCandidates(userID, text, "", "")
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return fmt.Errorf("delete: resolve candidates: %w", err)
	}

	switch len(candidates) {
	case 0:
		slog.InfoContext(ctx, "no candidates found", "user_id", userID)
		c.resolveMetric(ctx, userID, outcomeNoCandidates)
		c.sendText(ctx, b, chatID, msgNoCandidatesFound)
		return nil
	case 1:
		return c.startMovementDeleteFlowFor(ctx, b, chatID, userID, candidates, 0)
	default:
		return c.startMovementDeleteFlowFor(ctx, b, chatID, userID, candidates, -1)
	}
}

// startMovementDeleteFlowFor seeds and starts movement_delete.
// resolvedIndex >= 0 means exactly one candidate is already known (lets
// the flow skip its picker step); -1 means show the picker over every
// candidate.
func (c *controller) startMovementDeleteFlowFor(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, candidates []transactionGroup, resolvedIndex int) error {
	labels := make([]string, 0, len(candidates))
	for _, g := range candidates {
		labels = append(labels, candidateLabel(g))
	}

	seed := conversation.Data{
		keyCandidateLabels: encodeStringSlice(labels),
		keyCandidateGroups: encodeCandidateGroups(candidates),
	}
	if resolvedIndex >= 0 {
		seed[keyResolvedIndex] = strconv.Itoa(resolvedIndex)
	}

	prompt, err := c.engine.StartWithData(userID, movementDeleteFlowName, seed)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return fmt.Errorf("delete: start movement_delete flow: %w", err)
	}
	if b != nil {
		c.sendPrompt(ctx, b, chatID, prompt)
	}
	return nil
}

// candidateLabel builds the short display line shown per option in
// both UPDATE's and DELETE's ambiguous-candidate pickers.
func candidateLabel(g transactionGroup) string {
	if len(g.Movements) == 0 {
		return "?"
	}
	m := g.Movements[0]
	row := movementToRow(m)
	desc := row.Description
	if desc == "" {
		desc = row.Merchant
	}
	return movement.IconForType(m.Type) + " " + row.Amount + " " + row.Currency + " · " + desc + " · " + row.Date
}
