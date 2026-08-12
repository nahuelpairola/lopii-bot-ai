package messaging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
)

// needsFirstAccount is true when a non-transfer row has no account_id and the
// user has no default in that row's currency (zero-account first run) — the
// lazy-create trigger for stepCreateFirstAccount.
func needsFirstAccount(seed conversation.Data, has func(cur currency.Currency) bool) bool {
	return firstAccountCurrency(seed, has) != ""
}

// hasDefaultFor arma el predicado que needsFirstAccount/firstAccountCurrency
// piden. Existe porque los tres call sites del flujo lo escribían idéntico y
// nada garantizaba que siguieran de acuerdo.
func hasDefaultFor(accounts accountRepository, data conversation.Data) func(currency.Currency) bool {
	return func(cur currency.Currency) bool {
		return accounts.HasDefaultForCurrency(data.UserID(), cur)
	}
}

// firstAccountCurrency devuelve la moneda de la fila que dispara el alta, o ""
// si ninguna la dispara. Es la misma decisión que needsFirstAccount —de ahí que
// aquélla delegue acá— pero devolviendo el DATO en vez del sí/no.
//
// Hace falta porque el default de cuenta es POR MONEDA (FindDefaultByCurrency,
// UnsetDefault(userID, currency)). Un prompt que dice "tu cuenta principal" sin
// nombrar la moneda es ambiguo con dos cuentas y directamente falso con dos
// monedas: la nueva cuenta en USD no va a recibir ningún movimiento en pesos.
func firstAccountCurrency(seed conversation.Data, has func(cur currency.Currency) bool) string {
	for _, r := range decodeMovementRows(seed) {
		if movement.TypeFromString(r.Type) == movement.Transfer {
			continue
		}
		if r.AccountID == "" && !has(currency.Currency(r.Currency)) {
			return r.Currency
		}
	}
	return ""
}

// startMovementCreate runs Call 2 CREATE and either inserts directly
// (no gaps — the frictionless default) or starts movement_create seeded
// with whatever was resolved, landing on the first real gap.
func (c *controller) startMovementCreate(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) error {
	slog.InfoContext(ctx, "flow started", "flow", movementCreateFlowName, "user_id", userID)

	subs, err := c.subcategories.FindAllForUser(userID)
	if err != nil {
		c.sendText(ctx, b, chatID, msgCouldNotLoad)
		return fmt.Errorf("create: find subcategories: %w", err)
	}
	taxonomy := make([]orchestrator.TaxonomyEntry, 0, len(subs))
	for _, s := range subs {
		taxonomy = append(taxonomy, orchestrator.TaxonomyEntry{Category: s.Category, Subcategory: s.Subcategory, Description: s.Description})
	}

	accs, err := c.accounts.FindByUserID(userID)
	if err != nil {
		c.sendText(ctx, b, chatID, msgCouldNotLoad)
		return fmt.Errorf("create: find accounts: %w", err)
	}
	accountOptions := make([]orchestrator.AccountOption, 0, len(accs))
	for _, a := range accs {
		accountOptions = append(accountOptions, orchestrator.AccountOption{ID: uint64(a.ID), Name: a.Name, Currency: a.Currency.String()})
	}

	result, err := c.orchestrator.ClassifyCreate(ctx, text, taxonomy, accountOptions, todayCivil().Format("2006-01-02"))
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
		if handled, oerr := c.handleGroqError(ctx, b, chatID, userID, text, err); handled {
			return oerr
		}
		c.resolveMetric(ctx, userID, outcomeCreateFailed)
		c.sendText(ctx, b, chatID, msgSomethingBroke)
		return fmt.Errorf("create: classify: %w", err)
	}
	slog.DebugContext(ctx, "create classification result", "result", result)

	seed := buildCreateSeed(result, taxonomy)
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
				gateSeed[keyGatePrompt] = msgInsufficientFunds(short.shortfalls)
				return c.startFlow(ctx, b, chatID, userID, movementNegativeConfirmFlowName, gateSeed, "create: start negative-confirm flow")
			}
			c.resolveMetric(ctx, userID, outcomeCreateFailed)
			c.sendText(ctx, b, chatID, createErrorCopy(err))
			return fmt.Errorf("create insert (%s): %w", guardReason(err), err)
		}
		c.resolveMetric(ctx, userID, outcomeCreateInserted)
		c.sendText(ctx, b, chatID, msgConfirmMovements(inserted))
		return nil
	}

	return c.startFlow(ctx, b, chatID, userID, movementCreateFlowName, seed, "create: start movement_create flow")
}

// redirect* son los destinos de redirectTargetFor.
const (
	redirectAccount  = "account"
	redirectCategory = "category"
)

// redirectTargetFor detecta un mensaje que el router mandó a UPDATE/DELETE pero
// que en realidad pide algo sobre una CUENTA o una CATEGORÍA, no sobre un
// movimiento ("quiero dejar en cero algunas cuentas"). Devuelve "" cuando no hay
// que redirigir.
//
// Sin esta red el flujo va a buscar movimientos igual, y como resolveCandidates
// no dead-endea —cae al fallback de los N más recientes— el usuario termina
// viendo un picker de movimientos que no tienen nada que ver con lo que pidió.
//
// La regla es conservadora a propósito: si el mensaje nombra un movimiento, NO se
// redirige aunque también nombre una cuenta, porque "mover este movimiento a la
// cuenta de Mercado Pago" es un UPDATE legítimo. Preferimos no redirigir de más:
// un falso positivo manda al usuario a un flujo equivocado, un falso negativo
// solo lo deja como está hoy.
//
// ponytail: keywords, no LLM — es determinista, cuesta cero tokens y cero
// latencia. No cubre una cuenta nombrada sin la palabra "cuenta" ("cambiar el
// nombre del Fondo común de inversión Balanz"); eso pediría matchear contra los
// nombres de cuentas del usuario, con el riesgo de pisar un comercio homónimo.
func redirectTargetFor(message string) string {
	lower := strings.ToLower(message)
	if strings.Contains(lower, "movimiento") {
		return "" // nombra un movimiento: es UPDATE/DELETE de verdad
	}
	if strings.Contains(lower, "categoría") || strings.Contains(lower, "categoria") {
		return redirectCategory
	}
	if strings.Contains(lower, "cuenta") {
		return redirectAccount
	}
	return ""
}

// redirectMisroutedRequest desvía al flujo que corresponde un mensaje que el
// router mandó a UPDATE/DELETE pero que en realidad pide algo sobre una cuenta o
// una categoría. Devuelve handled=true si ya se ocupó del mensaje; el caller
// hace `return err` sin seguir con la resolución de candidatos. Mismo contrato
// que handleGroqError.
func (c *controller) redirectMisroutedRequest(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) (bool, error) {
	switch redirectTargetFor(text) {
	case redirectAccount:
		slog.InfoContext(ctx, "misrouted request redirected", "to", redirectAccount, "user_id", userID)
		return true, c.startAccountManage(ctx, b, chatID, userID, text)
	case redirectCategory:
		slog.InfoContext(ctx, "misrouted request redirected", "to", redirectCategory, "user_id", userID)
		return true, c.startCategoryManage(ctx, b, chatID, userID)
	}
	return false, nil
}

// startMovementUpdate resolves which existing movement(s) the message
// refers to via resolveCandidates (pg_trgm search, default 7-day
// window) and branches on how many candidates come back.
func (c *controller) startMovementUpdate(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) error {
	if handled, err := c.redirectMisroutedRequest(ctx, b, chatID, userID, text); handled {
		return err
	}
	slog.InfoContext(ctx, "flow started", "flow", movementUpdatePickFlowName, "user_id", userID)
	candidates, err := c.resolveCandidates(userID, text, "", "")
	if err != nil {
		c.sendText(ctx, b, chatID, msgCouldNotLoad)
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
		if err := c.proceedToUpdateConfirm(ctx, b, chatID, userID, text, candidates[0].TransactionID, oldIDs, rows, changeAsk{}); err != nil {
			if handled, oerr := c.handleGroqError(ctx, b, chatID, userID, text, err); handled {
				return oerr
			}
			c.sendText(ctx, b, chatID, msgSomethingBroke)
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
		return c.startFlow(ctx, b, chatID, userID, movementUpdatePickFlowName, seed, "update: start movement_update_pick flow")
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
	if handled, err := c.redirectMisroutedRequest(ctx, b, chatID, userID, text); handled {
		return err
	}
	slog.InfoContext(ctx, "flow started", "flow", movementDeleteFlowName, "user_id", userID)
	candidates, err := c.resolveCandidates(userID, text, "", "")
	if err != nil {
		c.sendText(ctx, b, chatID, msgCouldNotLoad)
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

	return c.startFlow(ctx, b, chatID, userID, movementDeleteFlowName, seed, "delete: start movement_delete flow")
}
