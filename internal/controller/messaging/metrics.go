package messaging

import (
	"context"
	"errors"
	"log/slog"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/trace"
)

// outcome* son los valores de intent_events.outcome. Los intents de
// movimiento arrancan en pending y se resuelven en su terminal; el resto es
// terminal directo.
const (
	outcomePending         = "pending"
	outcomeCreateInserted  = "create_inserted"
	outcomeCreateCancelled = "create_cancelled"
	outcomeCreateRewrite   = "create_rewrite"
	outcomeCreateFailed    = "create_failed"
	outcomeUpdateConfirmed = "update_confirmed"
	outcomeUpdateCancelled = "update_cancelled"

	// Los cuatro que reemplazan a update_failed. Se escribía desde SIETE
	// lugares con significados opuestos —el loop reventó, el parking falló, el
	// turno no hizo nada, y el usuario confirmó pero falló la escritura— y esa
	// sobrecarga hizo que el 2026-08-10 se diagnosticara mal: leímos "el loop
	// narró sin actuar" cuando en realidad la corrección había llegado a
	// confirmarse. El portón de la etapa lee esta columna.
	outcomeLoopErrored         = "loop_errored"     // la llamada al loop falló (transporte, no-429)
	outcomeParkFailed          = "park_failed"      // no se pudo guardar la acción parkeada
	outcomeLoopDidNothing      = "loop_did_nothing" // el turno no parkeó ni escribió nada
	outcomeWriteFailed         = "write_failed"     // el usuario confirmó y falló la escritura
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
func (c *controller) logIntent(ctx context.Context, userID uint64, rawMessage string, intent orchestrator.Intent, runErr error) {
	if c.metrics == nil {
		return
	}
	// needs_confirmation quedó vestigial (columna NOT NULL): se escribe false.
	if err := c.metrics.Log(userID, trace.ID(ctx), rawMessage, string(intent), false, initialOutcome(intent, runErr)); err != nil {
		slog.ErrorContext(ctx, "metric log intent failed", "err", err)
	}
}

// initialOutcome elige con qué outcome NACE el evento.
//
// CUALQUIER error lo deja `pending`, porque en los dos casos la historia sigue y
// alguien lo va a cerrar: un 429 lo cierra el drenaje cuando replaya; los demás
// los cierra el propio caller con `loop_errored`, dos líneas más abajo.
//
// Sin esto nacía `unclear`, que es TERMINAL, y entonces nadie encontraba un
// pendiente que resolver. Medido en vivo el 2026-08-12: "Cobré 500000 de sueldo"
// insertó $500.000 y quedó registrado como falla. Es la peor dirección posible
// para la columna que lee el portón — esconde los éxitos justo cuando el cupo
// aprieta.
func initialOutcome(intent orchestrator.Intent, runErr error) string {
	if runErr != nil {
		return outcomePending
	}
	return routerOutcome(intent)
}

// intentForExecutor traduce lo que hizo el loop al nombre de intent histórico,
// para que la serie de intent_events siga siendo comparable después de que el
// router desapareciera.
//
// Se usa el nombre EXISTENTE de cada intent, nunca un literal nuevo: un string
// inventado forkea la serie en silencio, que es exactamente lo que esta función
// existe para evitar.
func intentForExecutor(ex *agentExecutor, runErr error) orchestrator.Intent {
	var rateLimited *orchestrator.RateLimitedError
	switch {
	case errors.As(runErr, &rateLimited):
		// El cupo cortó antes de que el modelo eligiera herramienta: el intent no
		// se sabe todavía. UNCLEAR sería mentir —significa "no te entendí"— y con
		// el cupo apretado etiquetaría así a buena parte del tráfico. Lo completa
		// el drenaje cuando el replay funciona (SetIntentIfQueued).
		return orchestrator.IntentQueued
	case runErr != nil:
		return orchestrator.IntentUnclear
	case ex.answerQuery:
		return orchestrator.IntentQuery
	case ex.settingsArea == settingsAreaAccount:
		return orchestrator.IntentAccountManage
	case ex.settingsArea == settingsAreaCategory:
		return orchestrator.IntentCreateCategory
	case ex.settingsArea == settingsAreaCategoryManage:
		// Serie propia: administrar categorías tenía su intent antes del router,
		// y fusionarla con el alta escondería que son dos caminos con tasas de
		// éxito distintas.
		return orchestrator.IntentCategoryManage
	case ex.settingsArea == settingsAreaReminder:
		return orchestrator.IntentReminderSet
	case len(ex.inserted) > 0:
		return orchestrator.IntentCreate
	case ex.reply == msgHelp:
		return orchestrator.IntentHelp
	case ex.reply == msgAskRewrite:
		return orchestrator.IntentUnclear
	case len(ex.parked) > 0:
		// El tool del primero parkeado dice qué era. Los TRES casos importan: un
		// CREATE que se parkea para llenar un gap no insertó nada, así que la rama
		// de `inserted` no lo agarra y caía en el default de abajo — quedaba
		// contado como UPDATE. Medido en vivo el 2026-08-12 con "Lote cemento
		// 45000", que abrió un gap de cuenta y se registró como corrección.
		switch ex.parked[0].Tool {
		case orchestrator.ToolDeleteMovements:
			return orchestrator.IntentDelete
		case orchestrator.ToolRecordMovements:
			return orchestrator.IntentCreate
		default:
			return orchestrator.IntentUpdate
		}
	default:
		return orchestrator.IntentUnclear
	}
}

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
func writeOutcomeFor(data conversation.Data) string {
	if conversation.StringOrEmpty(data[conversation.KeyMode]) == modeUpdate {
		return outcomeUpdateConfirmed
	}
	return outcomeCreateInserted
}

func failureOutcomeFor(data conversation.Data) string {
	if conversation.StringOrEmpty(data[conversation.KeyMode]) == modeUpdate {
		return outcomeWriteFailed
	}
	return outcomeCreateFailed
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

// setQueuedIntent completa el intent de un evento que se encoló sin saberlo.
// Fire-and-forget, mismo criterio que logIntent: una métrica nunca rompe el
// flujo del usuario.
func (c *controller) setQueuedIntent(ctx context.Context, userID uint64, intent orchestrator.Intent) {
	if c.metrics == nil {
		return
	}
	if err := c.metrics.SetIntentIfQueued(userID, string(intent)); err != nil {
		slog.ErrorContext(ctx, "metric set queued intent failed", "err", err)
	}
}
