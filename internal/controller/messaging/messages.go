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
	msgUserCreatedSuccessfully     = "Bienvenid@ a LopiiBot, tus clasificador de gastos personales "

	msgGenericFlowError = "Algo salió mal, probá de nuevo en un momento."

	msgQueryNotSupported = "Todavía no puedo responder consultas — esa función está en camino. Mandame un movimiento para registrarlo, o una corrección/borrado de algo que ya cargaste."

	msgAccountSetupFinished = "Listo, ya podés empezar a registrar tus gastos. " +
		"Mandame algo como \"café 500\" o \"quiero crear una cuenta nueva\" si querés agregar otra cuenta más adelante."

	msgSubcategorySetupFinished = "Listo, tus subcategorías están guardadas. " +
		"Usá /subcategorias cuando quieras agregar más."
)

func createMsgUserDefaultAccountsCreatedSuccessfully(u *user.User, as []account.Account) string {
	accounts := make([]string, 0, len(as))

	for _, a := range as {
		accounts = append(accounts, fmt.Sprintf("%s (%s)", a.Name, a.Currency))
	}

	return fmt.Sprintf(
		"%s, disponés de %d cuentas: %s, donde se van a vincular todos tus movimientos por defecto.\n\n"+
			"Para arrancar con el saldo correcto, contame cuánta plata tenés hoy en cada una.",
		u.Username,
		len(as),
		strings.Join(accounts, ", "),
	)
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
	msgNoCandidatesFound = "No encontré ningún movimiento que coincida. Contame un poco más (comercio, monto o fecha)."
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
