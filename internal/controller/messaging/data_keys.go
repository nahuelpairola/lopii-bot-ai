package messaging

import "lopiibot.com/internal/conversation"

// dataKey is a conversation.Data map key, centralized so a key written in one
// flow and read in another can never drift on a typo. Alias (not a distinct
// type) so consts index conversation.Data with no cast.
type dataKey = string

const (
	// control flags (value "true"/"false"; used via flag/setFlag)
	keyCancelled        dataKey = "cancelled"
	keyConfirmed        dataKey = "confirmed"
	keySkipBalanceCheck dataKey = "_skip_balance_check"
	keyDeleteInstead    dataKey = "_delete_instead"
	keyEditProposal     dataKey = "edit_proposal" // Data key, NOT optionEditProposal
	keyCategoryIsNew    dataKey = "category_is_new"
	keyWeeklySummary    dataKey = "weekly_summary"
	keyAskDiscarded     dataKey = "_ask_discarded"

	// discriminators (key const; enum VALUE consts live in their owning file)
	keyMode        dataKey = "mode"
	keyOperation   dataKey = "operation"
	keyMatchChoice dataKey = "match_choice"
	keyMoveChoice  dataKey = "move_choice"

	// payload containers / queues
	keyMovements           dataKey = "movements"
	keyBeforeMovements     dataKey = "before_movements"
	keyOldMovementIDs      dataKey = "old_movement_ids"
	keyPendingCategoryGaps dataKey = "pending_category_gaps"
	keyPendingAccountGaps  dataKey = "pending_account_gaps"
	keyCandidateIDs        dataKey = "candidate_ids"
	keyCandidateLabels     dataKey = "candidate_labels"
	keyCandidateNames      dataKey = "candidate_names"
	keyCandidateCurrencies dataKey = "candidate_currencies"
	keyCandidateGroups     dataKey = "candidate_groups"
	keyResolvedIndex       dataKey = "resolved_index"

	// movementRow map fields (encode/decode pair)
	keyRowType          dataKey = "type"
	keyRowAmount        dataKey = "amount"
	keyCurrency         dataKey = "currency"
	keyAccountID        dataKey = "account_id" // shared: movementRow + account/manage flows
	keyAccountNameGuess dataKey = "account_name_guess"
	keyAccountName      dataKey = "account_name" // shared: movementRow + account flows + messages
	keyCategory         dataKey = "category"     // shared: movementRow + category_proposal + free_text
	keySubcategory      dataKey = "subcategory"
	keyPaymentMethod    dataKey = "payment_method"
	keyMerchant         dataKey = "merchant"
	keyDescription      dataKey = "description"
	keyDate             dataKey = "date"
	keyIcon             dataKey = "icon"
	keyGroup            dataKey = "group"

	// account create/manage flow keys
	keyAccountCurrency dataKey = "account_currency"
	keyAccountBalance  dataKey = "account_balance"
	keyNewName         dataKey = "new_name"
	keyNewTotal        dataKey = "new_total"
	keyMessage         dataKey = "message"

	// category proposal keys
	keyCategoryIcon           dataKey = "category_icon"
	keySubcategoryDescription dataKey = "subcategory_description"

	// account move-offer keys
	keyMoveFromID      dataKey = "move_from_id"
	keyMoveFromName    dataKey = "move_from_name"
	keyMoveFromBalance dataKey = "move_from_balance"
	keyMoveToID        dataKey = "move_to_id"
	keyMoveToName      dataKey = "move_to_name"

	// lazy-create (first account, movement_create flow) keys
	keyFirstAccountName    dataKey = "first_account_name"
	keyFirstAccountBalance dataKey = "first_account_balance"
	// keyFirstAccountCurrencies son las monedas de las cuentas que el alta
	// lazy-create acabó de crear. Lo escribe createFirstAccount y lo lee el
	// mensaje de confirmación, que para entonces ya no puede deducirlas: las
	// filas ya tienen account_id y la moneda que las originó se perdió.
	keyFirstAccountCurrencies dataKey = "first_account_currencies"

	// keyGapActiveRow guarda el índice de la fila que el gap-fill está
	// resolviendo ahora mismo: lo escribe el paso de categoría y lo leen el de
	// subcategoría y el prompt. Cruza tres archivos.
	keyGapActiveRow dataKey = "gap_active_row"

	// ask_user: la cola de preguntas abiertas de la acción parkeada que se está
	// drenando. keyOpenQuestions es UN string JSON ([]pendingaction.OpenQuestion),
	// no una lista, para cruzar JSONB sin desarmar mapas a mano.
	keyActionID      dataKey = "action_id"
	keyOpenQuestions dataKey = "open_questions"
	keyAskBudget     dataKey = "ask_budget"
	// keyAskRawAnswer es el buzón transitorio de TextStep: OnText lo lee, lo
	// archiva contra su pregunta y lo borra en la misma vuelta.
	keyAskRawAnswer dataKey = "_ask_raw_answer"

	// keyGatePrompt lleva el texto ya formateado del gate de saldo negativo.
	// Lo escribe free_text y lo lee movement_negative_confirm_flow, que no
	// tiene con qué recalcularlo.
	keyGatePrompt dataKey = "_gate_prompt"

	// category_manage: origen elegido en el flujo 1
	keySourceSubcategoryID dataKey = "source_subcategory_id"
	keySourceCategory      dataKey = "source_category"
	keySourceSubcategory   dataKey = "source_subcategory"

	// category_manage: contexto calculado entre los dos flujos
	keyMovementCount          dataKey = "movement_count"
	keySuggestedSubcategoryID dataKey = "suggested_subcategory_id"
	keySuggestedCategory      dataKey = "suggested_category"
	keySuggestedSubcategory   dataKey = "suggested_subcategory"

	// category_manage: destino elegido en el flujo 2
	keyTargetSubcategoryID dataKey = "target_subcategory_id"
	keyTargetCategory      dataKey = "target_category"
	keyTargetSubcategory   dataKey = "target_subcategory"
	// keyTargetOrigin guarda CÓMO se eligió el destino ("suggested"/"manual").
	// No es derivable comparando destino contra sugerencia: el usuario puede
	// elegir a mano exactamente la sugerida. Lo necesita el Atrás del confirm.
	keyTargetOrigin dataKey = "target_origin"
)

// flag reports whether a "true"/"false" string flag in Data is set to "true".
func flag(data conversation.Data, k dataKey) bool { return stringOrEmpty(data[k]) == "true" }

// setFlag marks a string flag "true" in Data.
func setFlag(data conversation.Data, k dataKey) { data[k] = "true" }
