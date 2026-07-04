package messaging

import (
	"context"
	"strconv"
	"time"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
)

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
func (c *controller) handleFreeText(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) {
	intent, err := c.orchestrator.ClassifyIntent(ctx, text)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}

	switch intent {
	case orchestrator.IntentQuery:
		c.sendText(ctx, b, chatID, msgQueryNotSupported)
	case orchestrator.IntentCreate:
		c.startMovementCreate(ctx, b, chatID, userID, text)
	case orchestrator.IntentUpdate:
		c.startMovementUpdate(ctx, b, chatID, userID, text)
	case orchestrator.IntentDelete:
		c.startMovementDelete(ctx, b, chatID, userID, text)
	default:
		c.sendText(ctx, b, chatID, msgGenericFlowError)
	}
}

// startMovementCreate runs Call 2 CREATE and either inserts directly
// (no gaps — the frictionless default) or starts movement_create
// seeded with whatever was resolved, landing on the first real gap.
func (c *controller) startMovementCreate(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) {
	subs, err := c.subcategories.FindAllForUser(userID)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}
	taxonomy := make([]orchestrator.TaxonomyEntry, 0, len(subs))
	for _, s := range subs {
		taxonomy = append(taxonomy, orchestrator.TaxonomyEntry{Category: s.Category, Subcategory: s.Subcategory, Description: s.Description})
	}

	accs, err := c.accounts.FindByUserID(userID)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}
	accountOptions := make([]orchestrator.AccountOption, 0, len(accs))
	for _, a := range accs {
		accountOptions = append(accountOptions, orchestrator.AccountOption{ID: uint64(a.ID), Name: a.Name, Currency: a.Currency.String()})
	}

	result, err := c.orchestrator.ClassifyCreate(ctx, text, taxonomy, accountOptions, time.Now().Format("2006-01-02"))
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}

	seed := buildCreateSeed(result)
	hasGaps := len(decodeStringSlice(seed, "pending_category_gaps")) > 0 || len(decodeStringSlice(seed, "pending_account_gaps")) > 0

	if !hasGaps {
		seed[conversation.UserIDKey] = userID
		inserted, err := c.resolveAndInsertMovements(seed)
		if err != nil {
			c.sendText(ctx, b, chatID, msgGenericFlowError)
			return
		}
		c.lastTransactions.Set(userID, inserted)
		c.sendText(ctx, b, chatID, msgConfirmMovements(inserted))
		return
	}

	prompt, err := c.engine.StartWithData(userID, movementCreateFlowName, seed)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}
	if b != nil {
		c.sendPrompt(ctx, b, chatID, prompt)
	}
}

// startMovementUpdate implements the spec's reference-resolution
// pipeline: check lastTransaction first (this check IS Call 2 UPDATE's
// call #1 — if it resolves, its result is already the corrected set,
// so seedAndStartUpdateConfirm is called directly, no second call);
// otherwise fall back to resolveCandidates and branch on how many
// candidates come back.
func (c *controller) startMovementUpdate(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) {
	dateFrom, dateTo := "", ""
	if last, ok := c.lastTransactions.Get(userID); ok {
		oldIDs := make([]string, 0, len(last))
		beforeRows := make([]movementRow, 0, len(last))
		drafts := make([]orchestrator.MovementDraft, 0, len(last))
		for _, m := range last {
			row := movementToRow(m)
			oldIDs = append(oldIDs, strconv.FormatUint(uint64(m.ID), 10))
			beforeRows = append(beforeRows, row)
			drafts = append(drafts, rowToDraft(row))
		}

		result, err := c.orchestrator.ResolveUpdate(ctx, text, orchestrator.MovementCandidate{Movements: drafts})
		if err == nil && result.Resolved {
			if err := c.seedAndStartUpdateConfirm(ctx, b, chatID, userID, oldIDs, beforeRows, result); err != nil {
				c.sendText(ctx, b, chatID, msgGenericFlowError)
			}
			return
		}
		if err == nil {
			dateFrom, dateTo = result.MentionedDateFrom, result.MentionedDateTo
		}
	}

	candidates, err := c.resolveCandidates(userID, text, dateFrom, dateTo)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}

	switch len(candidates) {
	case 0:
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
		}
	default:
		labels := make([]string, 0, len(candidates))
		for _, g := range candidates {
			labels = append(labels, candidateLabel(g))
		}
		seed := conversation.Data{
			"message":          text,
			"candidate_labels": encodeStringSlice(labels),
			"candidate_groups": encodeCandidateGroups(candidates),
		}
		prompt, err := c.engine.StartWithData(userID, movementUpdatePickFlowName, seed)
		if err != nil {
			c.sendText(ctx, b, chatID, msgGenericFlowError)
			return
		}
		if b != nil {
			c.sendPrompt(ctx, b, chatID, prompt)
		}
	}
}

// startMovementDelete mirrors startMovementUpdate's reference
// resolution, but — since deleting needs no second LLM call once a
// candidate is known (see movement_delete_flow.go) — it seeds
// movement_delete directly with resolved_index already set whenever
// there's exactly one candidate, letting the flow's Skip mechanism
// bypass the picker entirely.
func (c *controller) startMovementDelete(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) {
	dateFrom, dateTo := "", ""
	if last, ok := c.lastTransactions.Get(userID); ok {
		drafts := make([]orchestrator.MovementDraft, 0, len(last))
		for _, m := range last {
			drafts = append(drafts, rowToDraft(movementToRow(m)))
		}
		result, err := c.orchestrator.ResolveDelete(ctx, text, orchestrator.MovementCandidate{Movements: drafts})
		if err == nil && result.Resolved {
			c.startMovementDeleteFlowFor(ctx, b, chatID, userID, []transactionGroup{{Movements: last}}, 0)
			return
		}
		if err == nil {
			dateFrom, dateTo = result.MentionedDateFrom, result.MentionedDateTo
		}
	}

	candidates, err := c.resolveCandidates(userID, text, dateFrom, dateTo)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}

	switch len(candidates) {
	case 0:
		c.sendText(ctx, b, chatID, msgNoCandidatesFound)
	case 1:
		c.startMovementDeleteFlowFor(ctx, b, chatID, userID, candidates, 0)
	default:
		c.startMovementDeleteFlowFor(ctx, b, chatID, userID, candidates, -1)
	}
}

// startMovementDeleteFlowFor seeds and starts movement_delete.
// resolvedIndex >= 0 means exactly one candidate is already known (lets
// the flow skip its picker step); -1 means show the picker over every
// candidate.
func (c *controller) startMovementDeleteFlowFor(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, candidates []transactionGroup, resolvedIndex int) {
	labels := make([]string, 0, len(candidates))
	for _, g := range candidates {
		labels = append(labels, candidateLabel(g))
	}

	seed := conversation.Data{
		"candidate_labels": encodeStringSlice(labels),
		"candidate_groups": encodeCandidateGroups(candidates),
	}
	if resolvedIndex >= 0 {
		seed["resolved_index"] = strconv.Itoa(resolvedIndex)
	}

	prompt, err := c.engine.StartWithData(userID, movementDeleteFlowName, seed)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}
	if b != nil {
		c.sendPrompt(ctx, b, chatID, prompt)
	}
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
