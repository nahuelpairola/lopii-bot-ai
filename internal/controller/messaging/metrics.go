package messaging

import (
	"log"

	"lopiibot.com/internal/orchestrator"
)

// outcome* son los valores de intent_events.outcome. Los intents de
// movimiento arrancan en pending y se resuelven en su terminal; el resto es
// terminal directo.
const (
	outcomePending              = "pending"
	outcomeCreateInserted       = "create_inserted"
	outcomeCreateCancelled      = "create_cancelled"
	outcomeCreateRewrite        = "create_rewrite"
	outcomeUpdateConfirmed      = "update_confirmed"
	outcomeUpdateCancelled      = "update_cancelled"
	outcomeDeleteConfirmed      = "delete_confirmed"
	outcomeDeleteCancelled      = "delete_cancelled"
	outcomeNoCandidates         = "no_candidates"
	outcomeQueryUnsupported     = "query_unsupported"
	outcomeAccountCreateRouted  = "account_create_routed"
	outcomeCategoryCreateRouted = "category_create_routed"
)

// routerOutcome mapea el intent del router al outcome inicial que se loguea
// al clasificar. CREATE/UPDATE/DELETE arrancan pending (se resuelven en el
// terminal de su flow); el resto es terminal en el acto.
func routerOutcome(intent orchestrator.Intent) string {
	switch intent {
	case orchestrator.IntentCreate, orchestrator.IntentUpdate, orchestrator.IntentDelete:
		return outcomePending
	case orchestrator.IntentQuery:
		return outcomeQueryUnsupported
	case orchestrator.IntentAccountCreate:
		return outcomeAccountCreateRouted
	case orchestrator.IntentCreateCategory:
		return outcomeCategoryCreateRouted
	default:
		return outcomePending
	}
}

// logIntent registra la clasificación del router. Fire-and-forget: una
// escritura de métrica nunca rompe el flujo del usuario. El nil-guard
// mantiene verdes los tests que construyen el controller sin metrics.
func (c *controller) logIntent(userID uint64, rawMessage string, intent orchestrator.Intent, needsConfirmation bool) {
	if c.metrics == nil {
		return
	}
	if err := c.metrics.Log(userID, rawMessage, string(intent), needsConfirmation, routerOutcome(intent)); err != nil {
		log.Printf("metric: log intent: %v", err)
	}
}

// resolveMetric mueve el último pending del usuario a un outcome terminal.
// Fire-and-forget, mismo criterio que logIntent.
func (c *controller) resolveMetric(userID uint64, outcome string) {
	if c.metrics == nil {
		return
	}
	if err := c.metrics.Resolve(userID, outcome); err != nil {
		log.Printf("metric: resolve %s: %v", outcome, err)
	}
}
