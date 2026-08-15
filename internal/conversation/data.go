package conversation

import "maps"

// DataKey is a conversation.Data map key, centralized so a key written in one
// flow and read in another can never drift on a typo. Alias (not a distinct
// type) so consts index conversation.Data with no cast.
type DataKey = string

const (
	// Control flags (value "true"/"false"; used via Flag/SetFlag).
	KeyCancelled        DataKey = "cancelled"
	KeyConfirmed        DataKey = "confirmed"
	KeySkipBalanceCheck DataKey = "_skip_balance_check"
	KeyDeleteInstead    DataKey = "_delete_instead"
	KeyEditProposal     DataKey = "edit_proposal" // Data key, NOT optionEditProposal
	KeyCategoryIsNew    DataKey = "category_is_new"
	KeyWeeklySummary    DataKey = "weekly_summary"
	KeyAskDiscarded     DataKey = "_ask_discarded"

	// Discriminators (key const; enum VALUE consts live in their owning file).
	KeyMode        DataKey = "mode"
	KeyOperation   DataKey = "operation"
	KeyMatchChoice DataKey = "match_choice"
	KeyMoveChoice  DataKey = "move_choice"

	// Payload containers / queues.
	KeyMovements           DataKey = "movements"
	KeyBeforeMovements     DataKey = "before_movements"
	KeyOldMovementIDs      DataKey = "old_movement_ids"
	KeyPendingCategoryGaps DataKey = "pending_category_gaps"
	KeyPendingAccountGaps  DataKey = "pending_account_gaps"
	KeyCandidateIDs        DataKey = "candidate_ids"
	KeyCandidateLabels     DataKey = "candidate_labels"
	KeyCandidateNames      DataKey = "candidate_names"
	KeyCandidateCurrencies DataKey = "candidate_currencies"
	KeyCandidateGroups     DataKey = "candidate_groups"
	KeyResolvedIndex       DataKey = "resolved_index"

	// MovementRow map fields (encode/decode pair).
	KeyRowType          DataKey = "type"
	KeyRowAmount        DataKey = "amount"
	KeyCurrency         DataKey = "currency"
	KeyAccountID        DataKey = "account_id" // shared: movementRow + account/manage flows
	KeyAccountNameGuess DataKey = "account_name_guess"
	KeyAccountName      DataKey = "account_name" // shared: movementRow + account flows + messages
	KeyCategory         DataKey = "category"     // shared: movementRow + category_proposal + free_text
	KeySubcategory      DataKey = "subcategory"
	KeyPaymentMethod    DataKey = "payment_method"
	KeyDescription      DataKey = "description"
	KeyDate             DataKey = "date"
	KeyIcon             DataKey = "icon"
	KeyGroup            DataKey = "group"

	// Account create/manage flow keys.
	KeyAccountCurrency DataKey = "account_currency"
	KeyAccountBalance  DataKey = "account_balance"
	KeyNewName         DataKey = "new_name"
	KeyNewTotal        DataKey = "new_total"
	KeyMessage         DataKey = "message"

	// Category proposal keys.
	KeyCategoryIcon           DataKey = "category_icon"
	KeySubcategoryDescription DataKey = "subcategory_description"

	// Account move-offer keys.
	KeyMoveFromID      DataKey = "move_from_id"
	KeyMoveFromName    DataKey = "move_from_name"
	KeyMoveFromBalance DataKey = "move_from_balance"
	KeyMoveToID        DataKey = "move_to_id"
	KeyMoveToName      DataKey = "move_to_name"

	// Lazy-create (first account, movement_create flow) keys.
	KeyFirstAccountName    DataKey = "first_account_name"
	KeyFirstAccountBalance DataKey = "first_account_balance"
	// KeyFirstAccountCurrencies son las monedas de las cuentas que el alta
	// lazy-create acabó de crear. Lo escribe createFirstAccount y lo lee el
	// mensaje de confirmación, que para entonces ya no puede deducirlas: las
	// filas ya tienen account_id y la moneda que las originó se perdió.
	KeyFirstAccountCurrencies DataKey = "first_account_currencies"

	// KeyGapActiveRow guarda el índice de la fila que el gap-fill está
	// resolviendo ahora mismo: lo escribe el paso de categoría y lo leen el de
	// subcategoría y el prompt. Cruza tres archivos.
	KeyGapActiveRow DataKey = "gap_active_row"

	// Ask_user: la cola de preguntas abiertas de la acción parkeada que se está
	// drenando. KeyOpenQuestions es UN string JSON ([]pendingaction.OpenQuestion),
	// no una lista, para cruzar JSONB sin desarmar mapas a mano.
	KeyActionID      DataKey = "action_id"
	KeyOpenQuestions DataKey = "open_questions"
	KeyAskBudget     DataKey = "ask_budget"
	// KeyAskRawAnswer es el buzón transitorio de TextStep: OnText lo lee, lo
	// archiva contra su pregunta y lo borra en la misma vuelta.
	KeyAskRawAnswer DataKey = "_ask_raw_answer"

	// KeyGatePrompt lleva el texto ya formateado del gate de saldo negativo.
	// Lo escribe free_text y lo lee movement_negative_confirm_flow, que no
	// tiene con qué recalcularlo.
	KeyGatePrompt DataKey = "_gate_prompt"

	// Category_manage: origen elegido en el flujo 1.
	KeySourceSubcategoryID DataKey = "source_subcategory_id"
	KeySourceCategory      DataKey = "source_category"
	KeySourceSubcategory   DataKey = "source_subcategory"

	// Category_manage: contexto calculado entre los dos flujos.
	KeyMovementCount          DataKey = "movement_count"
	KeySuggestedSubcategoryID DataKey = "suggested_subcategory_id"
	KeySuggestedCategory      DataKey = "suggested_category"
	KeySuggestedSubcategory   DataKey = "suggested_subcategory"

	// Category_manage: destino elegido en el flujo 2.
	KeyTargetSubcategoryID DataKey = "target_subcategory_id"
	KeyTargetCategory      DataKey = "target_category"
	KeyTargetSubcategory   DataKey = "target_subcategory"
	// KeyTargetOrigin guarda CÓMO se eligió el destino ("suggested"/"manual").
	// No es derivable comparando destino contra sugerencia: el usuario puede
	// elegir a mano exactamente la sugerida. Lo necesita el Atrás del confirm.
	KeyTargetOrigin DataKey = "target_origin"
)

// Flag reports whether a "true"/"false" string flag in Data is set to "true".
func Flag(data Data, k DataKey) bool { return StringOrEmpty(data[k]) == "true" }

// SetFlag marks a string flag "true" in Data.
func SetFlag(data Data, k DataKey) { data[k] = "true" }

// StringOrEmpty reads a Data value as a string, tolerating the JSONB round-trip.
func StringOrEmpty(v any) string {
	s, _ := v.(string)
	return s
}

// CopyData es maps.Clone con una garantía extra: el resultado nunca es nil.
// maps.Clone(nil) devuelve nil y todos los call sites escriben sobre la copia,
// así que un nil silencioso acá sería un panic en la vuelta siguiente.
func CopyData(data Data) Data {
	if data == nil {
		return Data{}
	}
	return maps.Clone(data)
}

// DecodeStringSlice read a Data value that was stored as an []interface{} of
// strings (the JSONB shape) back into a []string.
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

// EncodeStringSlice stores a []string as the []interface{} shape that survives
// the JSONB round-trip.
func EncodeStringSlice(items []string) []interface{} {
	out := make([]interface{}, 0, len(items))
	for _, s := range items {
		out = append(out, s)
	}
	return out
}
