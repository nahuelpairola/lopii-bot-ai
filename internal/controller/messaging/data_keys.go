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

	// discriminators (key const; enum VALUE consts live in their owning file)
	keyMode        dataKey = "mode"
	keyOperation   dataKey = "operation"
	keyMatchChoice dataKey = "match_choice"
	keyMoveChoice  dataKey = "move_choice"

	// payload containers / queues
	keyMovements           dataKey = "movements"
	keyBeforeMovements     dataKey = "before_movements"
	keyAccounts            dataKey = "accounts"
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
	keyCurrency         dataKey = "currency"    // shared: movementRow + onboardingRow
	keyAccountID        dataKey = "account_id"  // shared: movementRow + account/manage flows
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

	// onboardingRow map fields (currency reuses keyCurrency)
	keyName      dataKey = "name"
	keyBalance   dataKey = "balance"
	keyIsDefault dataKey = "is_default"

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
)

// flag reports whether a "true"/"false" string flag in Data is set to "true".
func flag(data conversation.Data, k dataKey) bool { return stringOrEmpty(data[k]) == "true" }

// setFlag marks a string flag "true" in Data.
func setFlag(data conversation.Data, k dataKey) { data[k] = "true" }
