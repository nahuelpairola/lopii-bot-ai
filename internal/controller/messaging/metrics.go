package messaging

import (
	"context"
	"log/slog"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/movement"
)

// outcome* son los valores de intent_events.outcome. Los intents de
// movimiento arrancan en pending y se resuelven en su terminal; el resto es
// terminal directo.
const (
	outcomePending       = "pending"
	outcomeCreateRewrite = "create_rewrite"

	// Los cuatro que reemplazan a update_failed. Se escribía desde SIETE
	// lugares con significados opuestos —el loop reventó, el parking falló, el
	// turno no hizo nada, y el usuario confirmó pero falló la escritura— y esa
	// sobrecarga hizo que el 2026-08-10 se diagnosticara mal: leímos "el loop
	// narró sin actuar" cuando en realidad la corrección había llegado a
	// confirmarse. El portón de la etapa lee esta columna.
	outcomeLoopErrored         = "loop_errored"     // la llamada al loop falló (transporte, no-429)
	outcomeParkFailed          = "park_failed"      // no se pudo guardar la acción parkeada
	outcomeLoopDidNothing      = "loop_did_nothing" // el turno no parkeó ni escribió nada
	outcomeNoCandidates        = "no_candidates"
	outcomeQueryAnswered       = "query_answered"
	outcomeQueryFailed         = "query_failed"
	outcomeAccountCreateRouted = flow.OutcomeAccountCreateRouted
	outcomeReminderSetRouted   = "reminder_set_routed"
	outcomeHelpShown           = "help_shown"
	outcomeUnclear             = "unclear"
	outcomeCategoryMatchUsed   = flow.OutcomeCategoryMatchUsed
	outcomeCategoryCreated     = flow.OutcomeCategoryCreated
	outcomeCategoryCancelled   = flow.OutcomeCategoryCancelled

	outcomeAccountRenamed         = flow.OutcomeAccountRenamed
	outcomeAccountAdjusted        = flow.OutcomeAccountAdjusted
	outcomeAccountDefaultSet      = flow.OutcomeAccountDefaultSet
	outcomeAccountManageCancelled = flow.OutcomeAccountManageCancelled

	// outcomeCategoryManageNoOwn: el usuario pidió sacar una categoría pero no
	// creó ninguna. El bot entendió y respondió bien; no es una falla.
	outcomeCategoryManageNoOwn     = "category_manage_no_own"
	outcomeCategoryManageApplied   = flow.OutcomeCategoryManageApplied
	outcomeCategoryManageCancelled = flow.OutcomeCategoryManageCancelled
)

// Los outcomes de los flujos de movimiento viven en flow (movement_metrics.go);
// acá quedan los puentes que usan los caminos que todavía viven en el borde
// (agent_executor, start_movement, los tests). Un solo origen, dos nombres.
const (
	outcomeCreateInserted  = flow.OutcomeCreateInserted
	outcomeCreateCancelled = flow.OutcomeCreateCancelled
	outcomeCreateFailed    = flow.OutcomeCreateFailed
	outcomeUpdateConfirmed = flow.OutcomeUpdateConfirmed
	outcomeUpdateCancelled = flow.OutcomeUpdateCancelled
	outcomeWriteFailed     = flow.OutcomeWriteFailed
	outcomeDeleteConfirmed = flow.OutcomeDeleteConfirmed
	outcomeDeleteCancelled = flow.OutcomeDeleteCancelled
)

// writeOutcomeFor y failureOutcomeFor dicen si una escritura del flujo
// movement_create es un ALTA o una CORRECCIÓN.
//
// El mismo flow atiende las dos: una corrección que nombra una categoría que no
// existe se desvía acá para llenar el gap, y ahí escribía `create_inserted`
// SIEMPRE. Medido en vivo el 2026-08-12: "Era pollo" corrigió un movimiento y
// quedó registrado como alta nueva. Eso infla CREATE, borra UPDATE, y el portón
// de la etapa 5 lee exactamente esta columna — o sea que la métrica mentía justo
// en los casos que la etapa viene a arreglar.
//
// El seed ya traía el dato (`conversation.KeyMode`); nadie lo miraba.
//
// La lógica vive en flow (WriteOutcomeFor); el alias conserva el nombre corto
// para los callers del borde que aún no se migran.
func writeOutcomeFor(data conversation.Data) string {
	return flow.WriteOutcomeFor(data)
}

func failureOutcomeFor(data conversation.Data) string {
	return flow.FailureOutcomeFor(data)
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

// collectMovementIDs vive en flow (CollectMovementIDs); el alias conserva el
// nombre corto para los callers del borde que aún no se migran.
func collectMovementIDs(ms []movement.Movement) []uint {
	return flow.CollectMovementIDs(ms)
}
