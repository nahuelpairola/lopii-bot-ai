package messaging

import (
	"context"
	"log/slog"

	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/trace"
)

// outcome* son los valores de intent_events.outcome. Los intents de
// movimiento arrancan en pending y se resuelven en su terminal; el resto es
// terminal directo.
const (
	outcomePending             = "pending"
	outcomeCreateInserted      = "create_inserted"
	outcomeCreateCancelled     = "create_cancelled"
	outcomeCreateRewrite       = "create_rewrite"
	outcomeCreateFailed        = "create_failed"
	outcomeUpdateConfirmed     = "update_confirmed"
	outcomeUpdateCancelled     = "update_cancelled"
	outcomeUpdateFailed        = "update_failed"
	outcomeDeleteConfirmed     = "delete_confirmed"
	outcomeDeleteCancelled     = "delete_cancelled"
	outcomeNoCandidates        = "no_candidates"
	outcomeQueryAnswered       = "query_answered"
	outcomeQueryFailed         = "query_failed"
	outcomeAccountCreateRouted = "account_create_routed"
	outcomeReminderSetRouted   = "reminder_set_routed"
	outcomeHelpShown           = "help_shown"
	outcomeUnclear             = "unclear"
	outcomeCategoryMatchUsed   = "category_match_used"
	outcomeCategoryCreated     = "category_created"
	outcomeCategoryCancelled   = "category_create_cancelled"

	outcomeAccountRenamed         = "account_renamed"
	outcomeAccountAdjusted        = "account_adjusted"
	outcomeAccountDefaultSet      = "account_default_set"
	outcomeAccountManageCancelled = "account_manage_cancelled"

	// outcomeCategoryManageNoOwn: el usuario pidió sacar una categoría pero no
	// creó ninguna. El bot entendió y respondió bien; no es una falla.
	outcomeCategoryManageNoOwn     = "category_manage_no_own"
	outcomeCategoryManageApplied   = "category_manage_applied"
	outcomeCategoryManageCancelled = "category_manage_cancelled"
)

// routerOutcome mapea el intent del router al outcome inicial que se loguea
// al clasificar. CREATE/UPDATE/DELETE arrancan pending (se resuelven en el
// terminal de su flow); el resto es terminal en el acto.
func routerOutcome(intent orchestrator.Intent) string {
	switch intent {
	case orchestrator.IntentCreate, orchestrator.IntentUpdate, orchestrator.IntentDelete, orchestrator.IntentQuery, orchestrator.IntentCreateCategory, orchestrator.IntentAccountManage, orchestrator.IntentCategoryManage:
		return outcomePending
	case orchestrator.IntentReminderSet:
		return outcomeReminderSetRouted
	case orchestrator.IntentHelp:
		return outcomeHelpShown
	case orchestrator.IntentUnclear:
		return outcomeUnclear
	default:
		return outcomePending
	}
}

// logIntent registra la clasificación del router. Fire-and-forget: una
// escritura de métrica nunca rompe el flujo del usuario. El nil-guard
// mantiene verdes los tests que construyen el controller sin metrics.
func (c *controller) logIntent(ctx context.Context, userID uint64, rawMessage string, intent orchestrator.Intent, needsConfirmation bool) {
	if c.metrics == nil {
		return
	}
	if err := c.metrics.Log(userID, trace.ID(ctx), rawMessage, string(intent), needsConfirmation, routerOutcome(intent)); err != nil {
		slog.ErrorContext(ctx, "metric log intent failed", "err", err)
	}
}

// resolveMetric mueve el último pending del usuario a un outcome terminal.
// Fire-and-forget, mismo criterio que logIntent. ctx es solo para el trace_id
// del log: NO se pasa a Resolve — una escritura de métrica no debe ser
// cancelable por el ctx del caller.
func (c *controller) resolveMetric(ctx context.Context, userID uint64, outcome string, movementIDs ...uint) {
	if c.metrics == nil {
		return
	}
	if err := c.metrics.Resolve(userID, outcome, movementIDs); err != nil {
		slog.ErrorContext(ctx, "metric resolve failed", "outcome", outcome, "err", err)
	}
}

// collectMovementIDs pulls the primary keys of a resolved movement set for
// intent_events traceability (populated by GORM Create on insert/replace).
func collectMovementIDs(ms []movement.Movement) []uint {
	ids := make([]uint, len(ms))
	for i, m := range ms {
		ids[i] = m.ID
	}
	return ids
}
