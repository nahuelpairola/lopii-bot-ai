package messaging

import (
	"encoding/json"
	"time"

	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/pendingjob"
)

// Mensajes estáticos, sin variables.
const (
	// Menú de preguntas (tip recurrente).
	msgMenuButton = "Preguntame"
	msgMenuHeader = "¿Qué querés saber?"
	msgMenuNoData = "Todavía no tengo suficiente cargado para sacar cuentas. Seguí anotando y en unos días te muestro."

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

	// Ack de la cola de pending jobs (429 terminal de Groq). Nunca silencioso:
	// ackShortWaitThreshold decide cuál de las dos rinde (pending_jobs.go).
	msgAckShortWait   = "Dame un segundo, ya te lo cargo 🙌"
	msgAckLongWaitFmt = "Estoy sin cupo por ~%d min 🙏 lo cargo apenas se libere y te aviso."

	// msgQueuedBehindPending: distinto del ack del 429 (no repetir), plural implica
	// que ambos van juntos; sin jerga de cola/pendiente.
	msgQueuedBehindPending = "Ese también, ya te los cargo 🙌"

	// MsgAccountReset is exported so the admin reset endpoint
	// (controller/admin) can send it before re-firing onboarding — the
	// admin package can't reach unexported messaging strings.
	MsgAccountReset = "🔄 Reseteamos tu cuenta. Arrancamos de nuevo:"

	msgAmountUnclear     = flow.MsgAmountUnclear
	msgCurrencyMismatch  = flow.MsgCurrencyMismatch
	msgNoAccountCurrency = flow.MsgNoAccountCurrency
	msgMovementMalformed = flow.MsgMovementMalformed

	msgQueryFailed = "No pude resolver esa consulta ahora. Probá reformularla o intentá de nuevo en un momento."

	msgReminderDisabled  = flow.MsgReminderDisabled
	msgReminderCancelled = flow.MsgReminderCancelled
	msgReminderAllOff    = flow.MsgReminderAllOff
	msgReminderHubExit   = flow.MsgReminderHubExit

	msgWeeklySummaryOn  = flow.MsgWeeklySummaryOn
	msgWeeklySummaryOff = flow.MsgWeeklySummaryOff

	msgSubcategorySetupFinished = flow.MsgSubcategorySetupFinished

	msgNotUnderstood = flow.MsgNotUnderstood
)

// msgCouldNotSave vive en flow (MsgCouldNotSave); el alias conserva el nombre
// corto para los callers del borde que aún no se migran.
func msgCouldNotSave(cosa string) string {
	return flow.MsgCouldNotSave(cosa)
}

// msgJobGaveUp: el drain se rindió con un job (429 permanente). Reusa
// msgCouldNotSave — el caso es exactamente el suyo ("no se guardó nada") y así
// hereda el tono de los 5 mensajes por-significado en vez de inventar copy
// paralela. Para free_text echoa el texto para que el usuario copie y pegue.
func msgJobGaveUp(job pendingjob.PendingJob) string {
	if job.Kind != kindFreeText {
		return msgCouldNotSave("el cambio")
	}
	var p freeTextPayload
	_ = json.Unmarshal(job.Payload, &p)
	txt := p.Text
	if len([]rune(txt)) > 40 {
		txt = string([]rune(txt)[:40]) + "…"
	}
	return msgCouldNotSave("«" + txt + "»")
}

func msgCouldNotDelete(cosa string) string {
	return flow.MsgCouldNotDelete(cosa)
}

// msgReminderSet builds the set/edit receipt. startMin/endMin are minutes
// since midnight; shown as whole hours.
func msgReminderSet(startMin, endMin int) string {
	return flow.MsgReminderSet(startMin, endMin)
}

func msgFirstAccountDefault(name string, currencies []string) string {
	return flow.MsgFirstAccountDefault(name, currencies)
}

const msgInviteMoreAccounts = flow.MsgInviteMoreAccounts

// movementReceiptLine vive en flow (MovementReceiptLine). El alias conserva el
// nombre corto para los tests del borde que todavía lo ejercitan.
func movementReceiptLine(m movement.Movement) string {
	return flow.MovementReceiptLine(m)
}

// rowMoney formatea el monto de una fila para mostrárselo al usuario. La fila
// carga el monto como string (viaja por JSONB hacia conversation_states), así
// que hay que reparsearlo. Si no parsea se muestra crudo: un monto raro no debe
// romper el mensaje entero.
func rowMoney(r movement.MovementRow) string {
	amt, err := movement.ParseARAmount(r.Amount)
	if err != nil {
		return r.Amount + " " + currency.Currency(r.Currency).Label()
	}
	return currency.FormatMoney(amt.Abs(), currency.Currency(r.Currency))
}

// rowDate rinde la fecha de una fila en relativo ("hoy"/"ayer"/"04/07").
func rowDate(r movement.MovementRow) string {
	d, err := time.Parse("2006-01-02", r.Date)
	if err != nil {
		return r.Date
	}
	return movement.RelativeDate(d)
}

const (
	msgUpdateApplied   = flow.MsgUpdateApplied
	msgUpdateDeleted   = flow.MsgUpdateDeleted
	msgUpdateCancelled = flow.MsgUpdateCancelled
)

// changeFieldOptions son los botones de la pregunta de qué cambiar. NO incluyen
// el monto a propósito: ese se escribe derecho y así el caso común —que es el
// monto— se resuelve en un paso. Los otros tres encadenan una segunda pregunta,
// porque tocar "la categoría" dice el campo pero no el valor.
//
// Sin emojis: el texto del botón se concatena al pedido, y un emoji ahí es
// ruido.
func changeFieldOptions() []string {
	return []string{labelChangeCategory, labelChangeDate, labelChangeAccount}
}

// Las etiquetas son consts porque se usan dos veces: para dibujar los botones y
// para traducir la respuesta de vuelta a un campo.
const (
	labelChangeCategory = "La categoría"
	labelChangeDate     = "La fecha"
	labelChangeAccount  = "La cuenta"
)

// changeFieldForLabel traduce el botón tocado al campo que nombra.
//
// Vive PEGADO a changeFieldOptions a propósito: si las dos listas divergen, el
// botón deja de construir la corrección, el pedido se cae al camino del modelo
// y nada lo avisa.
func changeFieldForLabel(label string) changeField {
	switch label {
	case labelChangeCategory:
		return fieldCategory
	case labelChangeDate:
		return fieldDate
	case labelChangeAccount:
		return fieldAccount
	default:
		return ""
	}
}

// iconOrDefault falls back to the generic folder icon for any row whose
// Icon never got populated (shouldn't happen post-backfill, but a
// defensive default costs nothing — same fallback subcategory.Cache uses).
func iconOrDefault(icon string) string {
	if icon == "" {
		return "📂"
	}
	return icon
}

const (
	msgDeleteApplied   = flow.MsgDeleteApplied
	msgDeleteCancelled = flow.MsgDeleteCancelled
)

const (
	msgCreateCancelled = flow.MsgCreateCancelled
)

const (
	msgAccountCreateCancelled = flow.MsgAccountCreateCancelled
)

func msgAccountCreateSuccess(name, cur, balance string) string {
	return flow.MsgAccountCreateSuccess(name, cur, balance)
}

// msgFlowCancelled: "cancelaste, no escribí nada" — no es específico de
// cuentas ni categorías, cualquier flujo de gestión que se cancela lo usa.
const msgFlowCancelled = flow.MsgFlowCancelled
const msgAccountManageNoChange = flow.MsgAccountManageNoChange

const msgCategoryMatchUse = flow.MsgCategoryMatchUse

const msgResumeCancelled = "Cancelado ✅ — arrancá de nuevo cuando quieras."

const msgLogMissingFirst = flow.MsgLogMissingFirst

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
