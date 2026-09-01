package flow

import (
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
)

const (
	OutcomeCreateInserted  = "create_inserted"
	OutcomeCreateCancelled = "create_cancelled"
	OutcomeCreateFailed    = "create_failed"
	OutcomeUpdateConfirmed = "update_confirmed"
	OutcomeUpdateCancelled = "update_cancelled"
	OutcomeWriteFailed     = "write_failed"
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

func CollectMovementIDs(ms []movement.Movement) []uint {
	ids := make([]uint, len(ms))
	for i, m := range ms {
		ids[i] = m.ID
	}
	return ids
}
