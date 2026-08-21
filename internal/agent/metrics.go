package agent

import (
	"context"
	"errors"
	"log/slog"

	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messages"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/trace"
)

// outcome* son los outcomes locales del loop. Los de los flujos de movimiento
// viven en flow; los del borde (query, cuentas, categorías) viven en messaging.
// El portón de la etapa lee esta columna.
const (
	outcomeLoopErrored    = "loop_errored"     // la llamada al loop falló (transporte, no-429)
	outcomeParkFailed     = "park_failed"      // no se pudo guardar la acción parkeada
	outcomeLoopDidNothing = "loop_did_nothing" // el turno no parkeó ni escribió nada
	// Los dos de abajo salieron de loop_did_nothing porque NO son fracasos, y
	// mezclados con los que sí lo son la serie no se puede leer: "no cambié nada
	// porque ya estaba así" y "no cambié nada porque me niego" contaban igual que
	// "no cambié nada porque me rompí".
	outcomeNothingToChange   = "nothing_to_change"  // la corrección dejaba todo igual
	outcomeCorrectionRefused = "correction_refused" // una guarda la rechazó a propósito
	outcomeNoCandidates      = "no_candidates"
	outcomeHelpShown         = "help_shown"
	outcomeUnclear           = "unclear"
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

// outcomePending y outcomeReminderSetRouted viven en messaging (metrics.go del
// borde), que es de donde salen los terminales de los otros intents.
const (
	outcomePending           = "pending"
	outcomeReminderSetRouted = "reminder_set_routed"
)

// logIntent registra la clasificación del loop. Fire-and-forget: una
// escritura de métrica nunca rompe el flujo del usuario.
func logIntent(ctx context.Context, svc agentServices, userID uint64, rawMessage string, intent orchestrator.Intent, runErr error) {
	// needs_confirmation quedó vestigial (columna NOT NULL): se escribe false.
	if err := svc.MetricsLog(userID, trace.ID(ctx), rawMessage, string(intent), false, initialOutcome(intent, runErr)); err != nil {
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
	case ex.settingsArea == SettingsAreaAccount:
		return orchestrator.IntentAccountManage
	case ex.settingsArea == SettingsAreaCategory:
		return orchestrator.IntentCreateCategory
	case ex.settingsArea == SettingsAreaCategoryManage:
		// Serie propia: administrar categorías tenía su intent antes del router,
		// y fusionarla con el alta escondería que son dos caminos con tasas de
		// éxito distintas.
		return orchestrator.IntentCategoryManage
	case ex.settingsArea == SettingsAreaReminder:
		return orchestrator.IntentReminderSet
	case len(ex.inserted) > 0:
		return orchestrator.IntentCreate
	case ex.reply == messages.MsgHelp:
		return orchestrator.IntentHelp
	case ex.reply == messages.MsgAskRewrite:
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

// resolveMetric mueve el último pending del usuario a un outcome terminal.
// Fire-and-forget, mismo criterio que logIntent. ctx es solo para el trace_id
// del log: NO se pasa a Resolve — una escritura de métrica no debe ser
// cancelable por el ctx del caller.
func resolveMetric(ctx context.Context, svc agentServices, userID uint64, outcome string, movementIDs ...uint) {
	svc.ResolveMetric(ctx, userID, outcome, movementIDs...)
}

// collectMovementIDs vive en flow (CollectMovementIDs); el alias conserva el
// nombre corto para los callers del loop que aún no se migran.
func collectMovementIDs(ms []movement.Movement) []uint {
	return flow.CollectMovementIDs(ms)
}

// setQueuedIntent completa el intent de un evento que se encoló sin saberlo.
// Fire-and-forget, mismo criterio que logIntent: una métrica nunca rompe el
// flujo del usuario.
func setQueuedIntent(ctx context.Context, svc agentServices, userID uint64, intent orchestrator.Intent) {
	if err := svc.MetricsSetIntentIfQueued(userID, string(intent)); err != nil {
		slog.ErrorContext(ctx, "metric set queued intent failed", "err", err)
	}
}
