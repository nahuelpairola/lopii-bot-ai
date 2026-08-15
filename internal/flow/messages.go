package flow

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

// Copy de prompts de estado de flow (lo que el usuario ve mientras responde).
// El copy de resultado/edge queda en messaging — cada lado de la costura se
// queda con la mitad que le corresponde.

const (
	MsgInvalidChoice            = "Esa opción no está. Tocá un botón de abajo 👇"
	MsgInvalidAccountCreateName = "Mandame un nombre válido para la cuenta."

	MsgAskReminderWindow       = "¿En qué franja querés que te recuerde cargar los gastos? (solo te aviso los días que no anotaste nada)"
	MsgAskReminderCustomWindow = "Decime el rango en horario de 24 hs, por ejemplo: 20 a 21"
	MsgInvalidReminderWindow   = "No entendí el horario. Escribilo como \"20 a 21\" (en 24 hs, de menor a mayor)."
	MsgAskWeeklySummary        = "¿Querés que te mande un resumen de tu semana todos los lunes? 📊"
)

// GapPosition renders "(N de M)" para los prompts ask-category/subcategory/
// account — idx es 0-based, total es len(rows).
func GapPosition(idx, total int) string {
	return fmt.Sprintf(" (%d de %d)", idx+1, total)
}

func MsgAskCategory(data conversation.Data) string {
	rows := movement.DecodeMovementRows(data)
	idx, _ := strconv.Atoi(conversation.DecodeStringSlice(data, conversation.KeyPendingCategoryGaps)[0])
	return "¿A qué categoría pertenece " + movement.MovementGapDescriptor(rows[idx]) + "?" + GapPosition(idx, len(rows))
}

func MsgAskSubcategory(data conversation.Data) string {
	rows := movement.DecodeMovementRows(data)
	idx, _ := strconv.Atoi(conversation.StringOrEmpty(data[conversation.KeyGapActiveRow]))
	return "¿Y la subcategoría de " + movement.MovementGapDescriptor(rows[idx]) + ", dentro de " + rows[idx].Category + "?" + GapPosition(idx, len(rows))
}

func MsgAskAccount(data conversation.Data) string {
	rows := movement.DecodeMovementRows(data)
	idx, _ := strconv.Atoi(conversation.DecodeStringSlice(data, conversation.KeyPendingAccountGaps)[0])
	return "¿A qué cuenta corresponde " + movement.MovementGapDescriptor(rows[idx]) + "?" + GapPosition(idx, len(rows))
}

// Los mensajes del alta lazy-create nombran la MONEDA, porque el default de
// cuenta es por moneda. cur vacío (no debería pasar) cae en la redacción vieja.
func MsgAskFirstAccountName(cur string) string {
	if cur == "" {
		return "¿De dónde salió? Decime el nombre de la cuenta — ej: Galicia, Mercado Pago, efectivo."
	}
	return "¿De dónde salieron esos " + currency.Currency(cur).Label() + "? Decime el nombre de la cuenta — ej: Galicia, Mercado Pago, efectivo."
}

func MsgAskFirstAccountBalance(name, cur string) string {
	if cur == "" {
		return "¿Cuánto saldo tenés en " + name + " ahora? Poné el saldo que ves en tu cuenta (ej: 50000) — o mandá \"después\"."
	}
	return "¿Cuánto saldo tenés en " + name + " ahora, en " + currency.Currency(cur).Label() + "? Poné el saldo que ves en tu cuenta — o mandá \"después\"."
}

// RowMoney formatea el monto de una fila para mostrárselo al usuario. Si no
// parsea se muestra crudo: un monto raro no debe romper el mensaje entero.
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

// iconOrDefault falls back to the generic folder icon for any row whose Icon
// never got populated (defensive default — same fallback subcategory.Cache uses).
func iconOrDefault(icon string) string {
	if icon == "" {
		return "📂"
	}
	return icon
}

func MsgPickUpdateCandidate(conversation.Data) string {
	return "Encontré varios movimientos parecidos. ¿Cuál es?"
}

func MsgConfirmUpdateDiff(data conversation.Data) string {
	before := movement.DecodeMovementRows(conversation.Data{conversation.KeyMovements: data[conversation.KeyBeforeMovements]})

	// regalo/gratis total: the correction zeroes the movement, so it's a
	// deletion — show what will be removed, not a "corregiría a 0" diff.
	if conversation.Flag(data, conversation.KeyDeleteInstead) {
		lines := []string{"🗑️ Quedó gratis, así que lo voy a borrar:"}
		for _, b := range before {
			lines = append(lines, fmt.Sprintf("%s %s › %s — %s · %s (%s)",
				iconOrDefault(b.Icon), b.Category, b.Subcategory, rowMoney(b), b.Description, rowDate(b)))
		}
		return strings.Join(lines, "\n") + "\n\n¿Confirmás?"
	}

	after := movement.DecodeMovementRows(data)

	lines := []string{"✏️ Se corregiría así:"}
	for i, a := range after {
		var b movement.MovementRow
		if i < len(before) {
			b = before[i]
		}
		line := fmt.Sprintf("%s %s › %s — %s · %s (%s)",
			iconOrDefault(a.Icon), a.Category, a.Subcategory, rowMoney(a), a.Description, rowDate(a))
		if a.AccountName != "" {
			line += " · " + a.AccountName
		}
		changed := "antes: " + rowMoney(b)
		if b.AccountName != "" && b.AccountName != a.AccountName {
			changed += " · " + b.AccountName
		}
		line += " (" + changed + ")"
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n") + "\n\n¿Confirmás?"
}

func MsgPickDeleteCandidate(conversation.Data) string {
	return "Encontré varios movimientos parecidos. ¿Cuál querés borrar?"
}

func MsgConfirmDelete(data conversation.Data) string {
	idx, _ := strconv.Atoi(conversation.StringOrEmpty(data[conversation.KeyResolvedIndex]))
	candidates := DecodeCandidateGroups(data)
	if idx < 0 || idx >= len(candidates) {
		return "¿Confirmás el borrado?"
	}

	lines := []string{"🗑️ Se borraría:"}
	for _, row := range candidates[idx].Rows {
		// rowMoney y no "monto + código": el usuario lee "$3.000", no "3000 ARS".
		lines = append(lines, fmt.Sprintf("%s %s › %s — %s · %s (%s)",
			iconOrDefault(row.Icon), row.Category, row.Subcategory, rowMoney(row), row.Description, row.Date))
	}
	return strings.Join(lines, "\n") + "\n\n¿Confirmás?"
}

func MsgAskAccountCreateName(conversation.Data) string {
	return "¿Cómo querés llamar la cuenta nueva? (ej: Jubilación, Inversiones FCI)"
}

func MsgAskAccountCreateCurrency(conversation.Data) string {
	return "¿En qué moneda es la cuenta nueva?"
}

func MsgConfirmAccountCreate(data conversation.Data) string {
	name := conversation.StringOrEmpty(data[conversation.KeyAccountName])
	cur := conversation.StringOrEmpty(data[conversation.KeyAccountCurrency])
	balance := conversation.StringOrEmpty(data[conversation.KeyAccountBalance])
	return "Confirmá la cuenta nueva:\n\n" +
		"📛 Nombre: " + name + "\n" +
		"💱 Moneda: " + currency.Currency(cur).Label() + "\n" +
		"💰 Saldo inicial: " + balance + "\n\n" +
		"¿Confirmamos?"
}

// MsgCategoryMatchOffer se muestra cuando el LLM matcheó un CREATE_CATEGORY
// contra una entrada existente — ofrece reusarla en vez de duplicar.
func MsgCategoryMatchOffer(data conversation.Data) string {
	icon := conversation.StringOrEmpty(data[conversation.KeyCategoryIcon])
	if icon == "" {
		icon = "📂"
	}
	out := "Ya tenés una parecida: " + icon + " " + conversation.StringOrEmpty(data[conversation.KeyCategory]) + " › " + conversation.StringOrEmpty(data[conversation.KeySubcategory])
	if desc := conversation.StringOrEmpty(data[conversation.KeySubcategoryDescription]); desc != "" {
		out += "\n📝 " + desc
	}
	return out + "\n\n¿Te sirve o creás una distinta?"
}

// MsgCategoryProposalConfirm muestra la propuesta completa del LLM en un solo confirm.
func MsgCategoryProposalConfirm(data conversation.Data) string {
	icon := conversation.StringOrEmpty(data[conversation.KeyCategoryIcon])
	if icon == "" {
		icon = "📂"
	}
	line := icon + " " + conversation.StringOrEmpty(data[conversation.KeyCategory]) + " › " + conversation.StringOrEmpty(data[conversation.KeySubcategory])
	if conversation.Flag(data, conversation.KeyCategoryIsNew) {
		line += " (categoría nueva)"
	}
	return "Te propongo:\n" + line + "\n📝 " + conversation.StringOrEmpty(data[conversation.KeySubcategoryDescription]) + "\n\n¿La creo?"
}

// --- ACCOUNT_MANAGE ---
// Los saldos van por currency.FormatMoney, que conserva el signo — un saldo
// puede ser negativo y el usuario tiene que verlo tal cual.

func MsgAccountManagePick(conversation.Data) string {
	return "¿De cuál de tus cuentas me hablás? Elegila acá abajo y te muestro qué se puede hacer."
}

func MsgAccountManageMenu(name, cur string, balance decimal.Decimal) string {
	c := currency.Currency(cur)
	return fmt.Sprintf("Cuenta: %s (%s) — saldo actual %s.\n¿Qué querés hacer con ella?",
		name, c.Label(), currency.FormatMoney(balance, c))
}

func MsgAskAccountNewName(current string) string {
	return fmt.Sprintf("✏️ Decime el nombre nuevo para %s.\nEs el nombre con el que la vas a nombrar en tus mensajes.\n\nEj: FCI", current)
}

func MsgConfirmAccountRename(oldName, newName string) string {
	return fmt.Sprintf("Renombro %s → %s. ¿Confirmás?", oldName, newName)
}

func MsgAskAccountNewTotal(name string) string {
	return fmt.Sprintf("💰 Decime cuánto tenés en total hoy en %s.\nPoné el número que ves en tu banco o app.\nYo calculo la diferencia con lo registrado y la ajusto.\n\nEj: 52000", name)
}

// El renglón del medio no es decoración: el ajuste se registra como movimiento,
// y sin decirlo el usuario ve aparecer un "ingreso" que nunca cobró.
func MsgConfirmAccountAdjust(name, cur string, current, newTotal decimal.Decimal) string {
	delta := newTotal.Sub(current)
	sign := "+"
	if delta.IsNegative() {
		sign = "-"
	}
	c := currency.Currency(cur)
	return fmt.Sprintf("%s: %s → %s (ajuste %s%s)\n\nCorrige el saldo, no cuenta como gasto ni ingreso.\n¿Confirmás?",
		name, currency.FormatMoney(current, c), currency.FormatMoney(newTotal, c), sign, currency.FormatMoney(delta.Abs(), c))
}

func MsgConfirmAccountDefault(name, cur string) string {
	label := currency.Currency(cur).Label()
	return fmt.Sprintf("⭐ ¿%s pasa a ser tu cuenta en %s por defecto? Los movimientos en %s sin cuenta aclarada van a ir ahí.", name, label, label)
}

// MsgAskSubcategoryDescription es deliberadamente corto y concreto: la
// respuesta alimenta orchestrator.TaxonomyEntry.Description, la pista de
// clasificación del Call 2 CREATE.
func MsgAskSubcategoryDescription(sub string) string {
	return "En una frase: ¿cuándo se usa \"" + sub + "\"? (ej: \"gastos de comida y snacks en la calle\")"
}
