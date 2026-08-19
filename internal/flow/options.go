package flow

import "lopiibot.com/internal/conversation"

// Flow names, registered in server.go and read back by the edge
// (handleFlowFinished, FlowResumeLabel, start*). Exportados: el borde los usa
// para arrancar/continuar flows; los steps y opciones de cada flow quedan
// unexported en su propio archivo.
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

// Button values shared by every flow's gap-fill/confirm steps. Each is ONE
// const per meaning: the same literal can collide with a Data key without
// being the same thing (e.g. KeyEditProposal vs OptionEditProposal).
const (
	OptionCancel                = "cancel"
	OptionConfirm               = "confirm"
	OptionBalanceLater          = "balance_later"
	AccountChoiceExistingPrefix = "existing:"
	OptionAccountCreate         = "create"
	OptionBack                  = "back"
	OptionConfirmSeed           = "confirm_seed"
	OptionUseExisting           = "use_existing"
	OptionCreateNew             = "create_new"
	OptionEditProposal          = "edit_proposal"
)

// AccountPendingCreate es el valor centinela de MovementRow.AccountID que
// significa "el usuario eligió, a mitad de flow, crear esta cuenta" — los ids
// reales son strings numéricos, así que nunca colisiona.
const AccountPendingCreate = "PENDING_CREATE"

// MoveChoiceMove/MoveChoiceKeep son los valores del post-default del cambio
// de cuenta: "¿movés los movimientos del default viejo al nuevo?". Los lee
// finishAccountManageFlow (edge).
const (
	MoveChoiceMove = "move"
	MoveChoiceKeep = "keep"
)

// CancelOption is the "🚫 Cancelar" button appended to every gap-fill step's
// options — same escape hatch the movement_confirm_intent gate offers before
// the flow even starts, but for the case where the user only realizes
// mid-flow that the message was wrong.
var CancelOption = conversation.ChoiceOption{Label: "🚫 Cancelar", Value: OptionCancel, Finish: true}
