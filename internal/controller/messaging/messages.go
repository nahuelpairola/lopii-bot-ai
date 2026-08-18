package messaging

import (
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/movement"
)

// La copy del borde. Todo lo demás se fue con su cluster: los mensajes de
// estado de flow a internal/flow, el ack de la cola a internal/pendingjob, el
// menú de tips a internal/nudges. Acá queda lo que emite el edge mismo —
// onboarding, invitaciones y el gate de retomar— más los pocos alias que
// todavía tienen un caller de este lado.

// Mensajes estáticos, sin variables.
const (
	msgAlreadyHasAccount = "Ya tenés una cuenta activa. Mandame un gasto para registrarlo."
	msgPrivateBot        = "Este bot es privado. Si tenés una invitación, abrí el link que te compartieron."
	msgInvalidInvitation = "Esa invitación no es válida."
	msgInvitationError   = "Hubo un error procesando tu invitación, probá de nuevo en un momento."
	msgInvitationUsed    = "Esa invitación ya fue utilizada."
	msgInvitationExpired = "Esa invitación expiró, pedí una nueva."
	msgUserCreationError = "No pude crear tu cuenta, probá de nuevo."

	// MsgWelcome is exported so the admin reset endpoint (controller/admin)
	// can send it after clearing a user's flow state, without importing
	// unexported messaging symbols.
	MsgWelcome = "¡Hola! Soy Lopii 👋 Me hablás normal y yo anoto, corrijo y te respondo lo que preguntes.\n\n" +
		"Arrancá: mandame tu primer gasto — ej: \"gasté 500 en el súper\".\n\n" +
		"Cuando quieras, pedime \"ayuda\"."
	msgWelcome = MsgWelcome

	// Errores por-significado. Cada uno le dice al usuario de quién es la
	// culpa y qué pasó con su dato. Reemplazan al viejo msgGenericFlowError.
	msgCouldNotLoad   = flow.MsgCouldNotLoad
	msgSomethingBroke = flow.MsgSomethingBroke

	// MsgAccountReset is exported so the admin reset endpoint
	// (controller/admin) can send it before re-firing onboarding — the
	// admin package can't reach unexported messaging strings.
	MsgAccountReset = "🔄 Reseteamos tu cuenta. Arrancamos de nuevo:"

	msgQueryFailed = constants.QueryFailed

	msgResumeCancelled = "Cancelado ✅ — arrancá de nuevo cuando quieras."
)

// Alias con caller vivo en el borde: el finish de la primera cuenta y el
// recibo que ejercitan los tests de integración.
func msgFirstAccountDefault(name string, currencies []string) string {
	return flow.MsgFirstAccountDefault(name, currencies)
}

const msgInviteMoreAccounts = flow.MsgInviteMoreAccounts

func movementReceiptLine(m movement.Movement) string {
	return flow.MovementReceiptLine(m)
}

// FlowResumeLabel gives the resume gate (conversation.Engine) a short,
// per-flow description of what the user was doing, for its "¿retomamos o
// cancelamos?" prompt. One entry per registered flow; an unregistered or
// unrecognized name falls back to a generic phrase.
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
