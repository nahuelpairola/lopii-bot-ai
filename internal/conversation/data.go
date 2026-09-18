package conversation

import "maps"

type DataKey = string

const (
	KeyCancelled        DataKey = "cancelled"
	KeyConfirmed        DataKey = "confirmed"
	KeySkipBalanceCheck DataKey = "_skip_balance_check"
	KeyDeleteInstead    DataKey = "_delete_instead"
	KeyEditProposal     DataKey = "edit_proposal"
	KeyCategoryIsNew    DataKey = "category_is_new"
	KeyWeeklySummary    DataKey = "weekly_summary"
	KeyAskDiscarded     DataKey = "_ask_discarded"

	KeyMode        DataKey = "mode"
	KeyOperation   DataKey = "operation"
	KeyMatchChoice DataKey = "match_choice"
	KeyMoveChoice  DataKey = "move_choice"

	KeyMovements           DataKey = "movements"
	KeyBeforeMovements     DataKey = "before_movements"
	KeyOldMovementIDs      DataKey = "old_movement_ids"
	KeyPendingCategoryGaps DataKey = "pending_category_gaps"
	KeyPendingAccountGaps  DataKey = "pending_account_gaps"
	KeyPendingNewSubcats   DataKey = "pending_new_subcategories"
	KeyNewSubcategoryName  DataKey = "new_subcategory_name"
	KeyCandidateIDs        DataKey = "candidate_ids"
	KeyCandidateLabels     DataKey = "candidate_labels"
	KeyCandidateNames      DataKey = "candidate_names"
	KeyCandidateCurrencies DataKey = "candidate_currencies"
	KeyCandidateGroups     DataKey = "candidate_groups"
	KeyResolvedIndex       DataKey = "resolved_index"

	KeyRowType          DataKey = "type"
	KeyRowAmount        DataKey = "amount"
	KeyCurrency         DataKey = "currency"
	KeyAccountID        DataKey = "account_id"
	KeyAccountNameGuess DataKey = "account_name_guess"
	KeyAccountName      DataKey = "account_name"
	KeyCategory         DataKey = "category"
	KeySubcategory      DataKey = "subcategory"
	KeyPaymentMethod    DataKey = "payment_method"
	KeyDescription      DataKey = "description"
	KeyDate             DataKey = "date"
	KeyIcon             DataKey = "icon"
	KeyGroup            DataKey = "group"
	KeyTransferOut      DataKey = "transfer_out"

	KeyAccountCurrency DataKey = "account_currency"
	KeyAccountBalance  DataKey = "account_balance"
	KeyNewName         DataKey = "new_name"
	KeyNewTotal        DataKey = "new_total"
	KeyMessage         DataKey = "message"
	KeyOperationHint   DataKey = "operation_hint"

	KeyCategoryIcon           DataKey = "category_icon"
	KeySubcategoryDescription DataKey = "subcategory_description"

	KeyMoveFromID      DataKey = "move_from_id"
	KeyMoveFromName    DataKey = "move_from_name"
	KeyMoveFromBalance DataKey = "move_from_balance"
	KeyMoveToID        DataKey = "move_to_id"
	KeyMoveToName      DataKey = "move_to_name"

	KeyFirstAccountName       DataKey = "first_account_name"
	KeyFirstAccountBalance    DataKey = "first_account_balance"
	KeyFirstAccountCurrencies DataKey = "first_account_currencies"

	KeyGapActiveRow DataKey = "gap_active_row"

	KeyActionID      DataKey = "action_id"
	KeyOpenQuestions DataKey = "open_questions"
	KeyAskBudget     DataKey = "ask_budget"
	KeyAskRawAnswer  DataKey = "_ask_raw_answer"

	KeyGatePrompt DataKey = "_gate_prompt"

	KeySourceSubcategoryID DataKey = "source_subcategory_id"
	KeySourceCategory      DataKey = "source_category"
	KeySourceSubcategory   DataKey = "source_subcategory"

	KeyMovementCount          DataKey = "movement_count"
	KeySuggestedSubcategoryID DataKey = "suggested_subcategory_id"
	KeySuggestedCategory      DataKey = "suggested_category"
	KeySuggestedSubcategory   DataKey = "suggested_subcategory"

	KeyTargetSubcategoryID DataKey = "target_subcategory_id"
	KeyTargetCategory      DataKey = "target_category"
	KeyTargetSubcategory   DataKey = "target_subcategory"
	KeyTargetOrigin        DataKey = "target_origin"
)

func Flag(data Data, k DataKey) bool { return StringOrEmpty(data[k]) == "true" }

func SetFlag(data Data, k DataKey) { data[k] = "true" }

func StringOrEmpty(v any) string {
	s, _ := v.(string)
	return s
}

func CopyData(data Data) Data {
	if data == nil {
		return Data{}
	}
	return maps.Clone(data)
}

func DecodeStringSlice(data Data, key string) []string {
	raw, _ := data[key].([]interface{})
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func EncodeStringSlice(items []string) []interface{} {
	out := make([]interface{}, 0, len(items))
	for _, s := range items {
		out = append(out, s)
	}
	return out
}
