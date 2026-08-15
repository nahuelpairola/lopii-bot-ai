package flow

import (
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
)

// Outcome* son los valores de intent_events.outcome para los flujos de
// movimientos, cuentas y categorías. El resto (query, loop) sigue en
// messaging/metrics.go. Los valores son un contrato con la columna de DB:
// renombrarlos rompe las series históricas.
const (
	OutcomeCreateInserted  = "create_inserted"
	OutcomeCreateCancelled = "create_cancelled"
	OutcomeCreateFailed    = "create_failed"
	OutcomeUpdateConfirmed = "update_confirmed"
	OutcomeUpdateCancelled = "update_cancelled"
	OutcomeWriteFailed     = "write_failed" // el usuario confirmó y falló la escritura
	OutcomeDeleteConfirmed = "delete_confirmed"
	OutcomeDeleteCancelled = "delete_cancelled"

	OutcomeAccountCreateRouted    = "account_create_routed"
	OutcomeAccountRenamed         = "account_renamed"
	OutcomeAccountAdjusted        = "account_adjusted"
	OutcomeAccountDefaultSet      = "account_default_set"
	OutcomeAccountManageCancelled = "account_manage_cancelled"

	OutcomeCategoryMatchUsed       = "category_match_used"
	OutcomeCategoryCreated         = "category_created"
	OutcomeCategoryCancelled       = "category_create_cancelled"
	OutcomeCategoryManageCancelled = "category_manage_cancelled"
	OutcomeCategoryManageApplied   = "category_manage_applied"
)

// WriteOutcomeFor devuelve el outcome de éxito del finish según el modo con el
// que nació el flow (create vs update). Sustituye al approach de escribir el
// outcome del update sin mirar el modo, correcto solo por accidente: el seed ya
// traía el dato (`conversation.KeyMode`); nadie lo miraba.
func WriteOutcomeFor(data conversation.Data) string {
	if conversation.StringOrEmpty(data[conversation.KeyMode]) == ModeUpdate {
		return OutcomeUpdateConfirmed
	}
	return OutcomeCreateInserted
}

func FailureOutcomeFor(data conversation.Data) string {
	if conversation.StringOrEmpty(data[conversation.KeyMode]) == ModeUpdate {
		return OutcomeWriteFailed
	}
	return OutcomeCreateFailed
}

// CollectMovementIDs pulls the primary keys of a resolved movement set for
// intent_events traceability (populated by GORM Create on insert/replace).
func CollectMovementIDs(ms []movement.Movement) []uint {
	ids := make([]uint, len(ms))
	for i, m := range ms {
		ids[i] = m.ID
	}
	return ids
}
