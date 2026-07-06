package messaging

import (
	"fmt"
	"strconv"
	"strings"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/user"
)

// Mensajes estáticos, sin variables.
const (
	msgAlreadyHasAccount           = "Ya tenés una cuenta activa. Mandame un gasto para registrarlo."
	msgPrivateBot                  = "Este bot es privado. Si tenés una invitación, abrí el link que te compartieron."
	msgInvalidInvitation           = "Esa invitación no es válida."
	msgInvitationError             = "Hubo un error procesando tu invitación, probá de nuevo en un momento."
	msgInvitationUsed              = "Esa invitación ya fue utilizada."
	msgInvitationExpired           = "Esa invitación expiró, pedí una nueva."
	msgUserCreationError           = "No pude crear tu cuenta, probá de nuevo."
	msgDefaultAccountCreationError = "No se pudieron crear las cuentas por defecto"
	msgUserCreatedSuccessfully     = "¡Bienvenido/a! 👋 Soy Lopii, tu bot de finanzas.\n\n" +
		"Acá no hay formularios ni comandos: me hablás normal y yo entiendo. " +
		"A buen entendedor, pocas palabras 😉 Dame un segundo que te dejo todo listo."

	msgGenericFlowError = "Algo salió mal, probá de nuevo en un momento."

	msgQueryNotSupported = "Todavía no puedo responder consultas — esa función está en camino. Mandame un movimiento para registrarlo, o una corrección/borrado de algo que ya cargaste."

	msgAccountSetupFinished = "Listo, ya está 😎\n\n" +
		"Conmigo escribís lo justo. Tirás «café 500» y ya sé el resto: qué fue, cuánto y cuándo.\n\n" +
		"Así de fácil todo:\n" +
		"   «super 45mil con débito»\n" +
		"   «me equivoqué, eran 700»\n" +
		"   «cuánto gasté esta semana»\n\n" +
		"Poquito vos, el resto yo."

	msgSubcategorySetupFinished = "Listo, tu subcategoría está guardada ✅ " +
		"Mandame \"quiero crear otra categoría\" cuando quieras agregar más."
)

func createMsgUserDefaultAccountsCreatedSuccessfully(u *user.User, as []account.Account) string {
	return "Te armé dos cuentas para arrancar: una en pesos 🇦🇷 y una en dólares 🇺🇸.\n\n" +
		"Una cuenta es simplemente dónde tenés tu plata. Estas son tus principales: " +
		"cuando cargues un gasto, va acá solo, sin que me digas nada. " +
		"Más adelante sumás las que quieras (una inversión, ahorros, lo que sea).\n\n" +
		"Para arrancar con tus números reales, decime cuánta plata tenés hoy en cada una."
}

func msgConfirmInitialBalances(data conversation.Data) string {
	lines := make([]string, 0, len(currency.SupportedCurrencies))
	for _, cu := range currency.SupportedCurrencies {
		amount, _ := data[balanceDataKey(cu)].(string)
		lines = append(lines, fmt.Sprintf("• %s %s: %s", constants.DefaultWalletName, cu.String(), amount))
	}
	return fmt.Sprintf(
		"Así quedarían tus saldos iniciales:\n%s\n\n¿Confirmás o querés corregir?",
		strings.Join(lines, "\n"),
	)
}

func msgAskCategory(data conversation.Data) string {
	return "¿A qué categoría pertenece este movimiento?"
}

func msgAskSubcategory(data conversation.Data) string {
	return "¿Y la subcategoría?"
}

func msgAskAccount(data conversation.Data) string {
	return "¿A qué cuenta corresponde este movimiento?"
}

func msgConfirmMovements(movements []movement.Movement) string {
	lines := make([]string, 0, len(movements))
	for _, m := range movements {
		lines = append(lines, movementReceiptLine(m))
	}
	return "✅ Movimiento registrado\n" + strings.Join(lines, "\n")
}

// movementReceiptLine formats one movement for a receipt/confirmation
// message: icon, category › subcategory, amount, currency, description,
// and date — enough to tell movements apart at a glance when several
// look similar. Category/subcategory come from Movement.Subcategory
// (populated at construction time — see movement_create_flow.go — or via
// Preload for DB-fetched candidates — see movement/repository.go).
func movementReceiptLine(m movement.Movement) string {
	category, sub := "", ""
	if m.Subcategory != nil {
		category, sub = m.Subcategory.Category, m.Subcategory.Subcategory
	}
	desc := ""
	if m.Description != nil {
		desc = *m.Description
	}
	return fmt.Sprintf("%s %s › %s — %s %s · %s (%s)",
		movement.IconForType(m.Type), category, sub, m.Amount.String(), m.Currency.String(), desc, m.Date.Format("2006-01-02"))
}

func msgPickUpdateCandidate(data conversation.Data) string {
	return "Encontré varios movimientos parecidos. ¿Cuál es?"
}

func msgConfirmUpdateDiff(data conversation.Data) string {
	before := decodeMovementRows(conversation.Data{"movements": data["before_movements"]})
	after := decodeMovementRows(data)

	lines := []string{"✏️ Se corregiría así:"}
	for i, a := range after {
		var b movementRow
		if i < len(before) {
			b = before[i]
		}
		lines = append(lines, fmt.Sprintf("%s %s › %s — %s %s · %s (%s) (antes: %s %s)",
			iconOrDefault(a.Icon), a.Category, a.Subcategory, a.Amount, a.Currency, a.Description, a.Date, b.Amount, b.Currency))
	}
	return strings.Join(lines, "\n") + "\n\n¿Confirmás?"
}

const (
	msgUpdateApplied     = "✅ Corregido."
	msgUpdateCancelled   = "Cancelado, no cambié nada."
	// Shown only when the window (today, or the mentioned day) has no
	// movements at all — the fallback picker covers every other case.
	msgNoCandidatesFound = "No tengo movimientos de ese día para tocar. ¿De qué fecha era?"
)

func msgPickDeleteCandidate(data conversation.Data) string {
	return "Encontré varios movimientos parecidos. ¿Cuál querés borrar?"
}

func msgConfirmDelete(data conversation.Data) string {
	idx, _ := strconv.Atoi(stringOrEmpty(data["resolved_index"]))
	candidates := decodeCandidateGroups(data)
	if idx < 0 || idx >= len(candidates) {
		return "¿Confirmás el borrado?"
	}

	lines := []string{"🗑️ Se borraría:"}
	for _, row := range candidates[idx].Rows {
		lines = append(lines, fmt.Sprintf("%s %s › %s — %s %s · %s (%s)",
			iconOrDefault(row.Icon), row.Category, row.Subcategory, row.Amount, row.Currency, row.Description, row.Date))
	}
	return strings.Join(lines, "\n") + "\n\n¿Confirmás?"
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
	msgDeleteApplied   = "🗑️ Borrado."
	msgDeleteCancelled = "Cancelado, no borré nada."
)

func msgConfirmIntentUnclear(data conversation.Data) string {
	return "🤔 No estoy seguro de qué es este mensaje. ¿Qué preferís?"
}

const (
	msgAskRewrite             = "✍️ Dale, mandalo de nuevo con más detalle (monto, categoría, y si es un movimiento nuevo)."
	msgConfirmIntentCancelled = "🚫 Cancelado, no hice nada."
	msgCreateCancelled        = "🚫 Cancelado, no registré nada."
)

const (
	msgInvalidAccountCreateName = "Mandame un nombre válido para la cuenta."
	msgAccountCreateCancelled   = "🚫 Cancelado, no se creó ninguna cuenta."
)

func msgAskAccountCreateName(data conversation.Data) string {
	return "¿Cómo querés llamar la cuenta nueva? (ej: Jubilación, Inversiones FCI)"
}

func msgAskAccountCreateCurrency(data conversation.Data) string {
	return "¿En qué moneda es la cuenta nueva?"
}

func msgConfirmAccountCreate(data conversation.Data) string {
	name := stringOrEmpty(data["account_name"])
	cur := stringOrEmpty(data["account_currency"])
	balance := stringOrEmpty(data["account_balance"])
	return "Confirmá la cuenta nueva:\n\n" +
		"📛 Nombre: " + name + "\n" +
		"💱 Moneda: " + cur + "\n" +
		"💰 Saldo inicial: " + balance + "\n\n" +
		"¿Confirmamos?"
}

func msgAccountCreateSuccess(name, cur, balance string) string {
	return "✅ Cuenta \"" + name + "\" creada en " + cur + " con saldo inicial " + balance + "."
}

// msgAskSubcategoryDescription is deliberately short and concrete: the
// answer feeds orchestrator.TaxonomyEntry.Description, Call 2 CREATE's
// classification hint, so it must tell the LLM when/what this
// subcategory refers to — not just be a decorative label.
func msgAskSubcategoryDescription(sub string) string {
	return "En una frase: ¿cuándo se usa \"" + sub + "\"? (ej: \"gastos de comida y snacks en la calle\")"
}

const msgResumeCancelled = "Cancelado ✅ — arrancá de nuevo cuando quieras."

// FlowResumeLabel gives the resume gate (conversation.Engine) a short,
// per-flow description of what the user was doing, for its "¿retomamos o
// cancelamos?" prompt. One entry per registered flow; an unregistered or
// unrecognized name falls back to a generic phrase.
func FlowResumeLabel(flowName string) string {
	switch flowName {
	case movementCreateFlowName, movementConfirmFlowName:
		return "estabas registrando un movimiento"
	case movementUpdatePickFlowName, movementUpdateConfirmFlowName:
		return "estabas corrigiendo un movimiento"
	case movementDeleteFlowName:
		return "estabas borrando un movimiento"
	case accountCreateFlowName:
		return "estabas creando una cuenta"
	case subcategorySetupFlowName:
		return "estabas creando una subcategoría"
	case initialBalanceFlowName:
		return "estabas cargando tus saldos iniciales"
	default:
		return "una conversación anterior"
	}
}
