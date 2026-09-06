package flow

import "lopiibot.com/internal/conversation"

const (
	AskUserFlowName                 = "ask_user"
	AccountCreateFlowName           = "account_create"
	AccountManageFlowName           = "account_manage"
	AccountMoveOfferFlowName        = "account_move_offer"
	CategoryMatchOfferFlowName      = "category_match_offer"
	CategoryProposalConfirmFlowName = "category_proposal_confirm"
	CategoryManagePickFlowName      = "category_manage_pick"
	CategoryManageTargetFlowName    = "category_manage_target"
	MovementCreateFlowName          = "movement_create"
	MovementUpdatePickFlowName      = "movement_update_pick"
	MovementUpdateConfirmFlowName   = "movement_update_confirm"
	MovementDeleteFlowName          = "movement_delete"
	MovementNegativeConfirmFlowName = "movement_negative_confirm"
	ReminderSetupFlowName           = "reminder_setup"
	SubcategorySetupFlowName        = "subcategory_setup"
)

const (
	OptionCancel                = "cancel"
	OptionConfirm               = "confirm"
	OptionBalanceLater          = "balance_later"
	AccountChoiceExistingPrefix = "existing:"
	OptionAccountCreate         = "create"
	OptionSubcategoryCreate     = "create_subcategory"
	OptionBack                  = "back"
	OptionConfirmSeed           = "confirm_seed"
	OptionUseExisting           = "use_existing"
	OptionCreateNew             = "create_new"
	OptionEditProposal          = "edit_proposal"
)

const AccountPendingCreate = "PENDING_CREATE"

const MaxSubcategoryNameRunes = 30

const (
	MoveChoiceMove = "move"
	MoveChoiceKeep = "keep"
)

var CancelOption = conversation.ChoiceOption{Label: "🚫 Cancelar", Value: OptionCancel, Finish: true}
