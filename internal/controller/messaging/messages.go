package messaging

import (
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/flow"
)

const (
	msgAlreadyHasAccount = "Ya tenés una cuenta activa. Mandame un gasto para registrarlo."
	msgPrivateBot        = "Este bot es privado. Si tenés una invitación, abrí el link que te compartieron."
	msgInvalidInvitation = "Esa invitación no es válida."
	msgInvitationError   = "Hubo un error procesando tu invitación, probá de nuevo en un momento."
	msgInvitationUsed    = "Esa invitación ya fue utilizada."
	msgInvitationExpired = "Esa invitación expiró, pedí una nueva."
	msgUserCreationError = "No pude crear tu cuenta, probá de nuevo."

	MsgWelcome = "¡Hola! Soy Lopii 👋 Me hablás normal y yo anoto, corrijo y te respondo lo que preguntes.\n\n" +
		"Arrancá: mandame tu primer gasto — ej: \"gasté 500 en el súper\".\n\n" +
		"Cuando quieras, pedime \"ayuda\"."
	msgWelcome = MsgWelcome

	msgCouldNotLoad   = flow.MsgCouldNotLoad
	msgSomethingBroke = flow.MsgSomethingBroke

	MsgAccountReset = "🔄 Reseteamos tu cuenta. Arrancamos de nuevo:"

	msgQueryFailed = constants.QueryFailed

	msgResumeCancelled = "Cancelado ✅ — arrancá de nuevo cuando quieras."
)

const msgInviteMoreAccounts = flow.MsgInviteMoreAccounts

func FlowResumeLabel(flowName string) string {
	switch flowName {
	case flow.MovementCreateFlowName:
		return "estabas registrando un movimiento"
	case flow.MovementUpdatePickFlowName, flow.MovementUpdateConfirmFlowName:
		return "estabas corrigiendo un movimiento"
	case flow.MovementDeleteFlowName:
		return "estabas borrando un movimiento"
	case flow.AccountCreateFlowName:
		return "estabas creando una cuenta"
	case flow.SubcategorySetupFlowName:
		return "estabas creando una subcategoría"
	case flow.CategoryMatchOfferFlowName, flow.CategoryProposalConfirmFlowName:
		return "estabas creando una categoría"
	case flow.MovementNegativeConfirmFlowName:
		return "estabas confirmando un movimiento"
	case flow.AskUserFlowName:
		return "algo que te pregunté"
	default:
		return "una conversación anterior"
	}
}
