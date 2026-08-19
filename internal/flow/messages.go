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

// --- Copy de RESULTADO de los finishes de movimientos ---
// La consumen los finishes (movement_finish.go). El borde que todavía vive en
// messaging la re-exporta con nombres cortos (messages.go) hasta que se migre;
// por eso el copy es un contrato estable, no un detalle interno de flow.

const (
	MsgSomethingBroke     = "Se me complicó algo de mi lado, no es por vos. Probá de nuevo."
	MsgNotUnderstood      = "No te entendí 🤔 Probá de nuevo."
	MsgAmountUnclear      = "No entendí el monto 🤔 ¿Lo reescribís?"
	MsgCurrencyMismatch   = "Esa cuenta es de otra moneda. Reescribí el movimiento."
	MsgNoAccountCurrency  = "No tenés una cuenta en esa moneda. Creá una primero."
	MsgMovementMalformed  = "No pude armar ese movimiento. Reescribilo, porfa."
	MsgUpdateApplied      = "✅ Corregido."
	MsgUpdateDeleted      = "🗑️ Listo, lo borré (quedó gratis)."
	MsgUpdateCancelled    = "Cancelado, no cambié nada."
	MsgDeleteApplied      = "🗑️ Borrado."
	MsgDeleteCancelled    = "Cancelado, no borré nada."
	MsgCreateCancelled    = "🚫 Cancelado, no registré nada."
	MsgInviteMoreAccounts = "Podés tener más cuentas (inversiones, dólares, lo que sea). Decime \"creá una cuenta\" cuando quieras."
	MsgLogMissingFirst    = "Dale, registrá primero lo que falta y volvé a mandarme esto."

	MsgFlowCancelled          = "Listo, no toqué nada."
	MsgCouldNotLoad           = "No pude traer tus datos ahora. Probá en un momento."
	MsgAccountCreateCancelled = "🚫 Cancelado, no se creó ninguna cuenta."
	MsgAccountManageNoChange  = "Ya tenías ese saldo, no cambié nada."

	MsgSubcategorySetupFinished = "Listo, tu subcategoría está guardada ✅ " +
		"Mandame \"quiero crear otra categoría\" cuando quieras agregar más."
	MsgCategoryMatchUse = "Listo ✅ — registrá el gasto nombrándolo y cae ahí solo (ej: \"gasté 5000 en un regalo\")."

	MsgReminderDisabled  = "Dale, no te jodo más con eso 👍 Si querés que vuelva, avisame cuando quieras."
	MsgReminderCancelled = "Listo, dejé todo como estaba 👌"
	MsgReminderAllOff    = "🔕 Listo, apagué todas las notificaciones: ni recordatorio diario ni resumen semanal. Cuando quieras algo de vuelta, escribime \"notificaciones\"."
	MsgReminderHubExit   = "Listo 👌 Dejé todo como estaba."
	MsgWeeklySummaryOn   = "📊 Listo, te mando el resumen todos los lunes."
	MsgWeeklySummaryOff  = "📊 Ok, no te mando el resumen semanal."

	MsgNearDupSeparate = "Perfecto, los dejo separados."
	MsgNearDupMerged   = "Listo, quedó uno solo."
)

// MsgCouldNotSave nombra qué no quedó guardado, para que el usuario sepa que
// su acción no se registró. cosa: "tu movimiento", "tu cuenta", "el cambio", etc.
func MsgCouldNotSave(cosa string) string {
	return "No pude guardar " + cosa + ". No se guardó nada, probá de nuevo."
}

// MsgCouldNotDelete es el gemelo de MsgCouldNotSave para borrados: el
// reaseguro es inverso — la cosa sigue existiendo, no desapareció a medias.
func MsgCouldNotDelete(cosa string) string {
	return "No pude borrar " + cosa + ". Sigue ahí, probá de nuevo."
}

func MsgAccountCreateSuccess(name, cur, balance string) string {
	return "✅ Cuenta \"" + name + "\" creada en " + currency.Currency(cur).Label() + " con saldo inicial " + balance + "."
}

// MsgReminderSet builds the set/edit receipt. startMin/endMin are minutes
// since midnight; shown as whole hours.
func MsgReminderSet(startMin, endMin int) string {
	return fmt.Sprintf("Listo 🙌 Te recuerdo cargar gastos entre las %d y las %d, solo los días que no hayas anotado nada.", startMin/60, endMin/60)
}

// MsgFirstAccountDefault nombra las monedas de las cuentas recién creadas. Son
// varias cuando un mismo mensaje trae filas en dos monedas: CreateFirstAccount
// crea UNA CUENTA POR MONEDA, todas con el nombre que dio el usuario.
func MsgFirstAccountDefault(name string, currencies []string) string {
	labels := make([]string, 0, len(currencies))
	for _, c := range currencies {
		labels = append(labels, currency.Currency(c).Label())
	}
	switch len(labels) {
	case 0:
		return "⭐ Dejé " + name + " como tu cuenta principal — la uso cuando no me aclarás de dónde sale la plata."
	case 1:
		return "⭐ Dejé " + name + " como tu cuenta en " + labels[0] + " por defecto — la uso para los movimientos en " + labels[0] + " cuando no me aclarás de dónde sale la plata."
	default:
		list := strings.Join(labels[:len(labels)-1], ", ") + " y " + labels[len(labels)-1]
		return "⭐ Creé " + name + " en " + list + ", y las dejé por defecto para cada una — las uso cuando no me aclarás de dónde sale la plata."
	}
}

// MsgConfirmMovements es el recibo de un CREATE: una línea por movimiento.
func MsgConfirmMovements(movements []movement.Movement) string {
	lines := make([]string, 0, len(movements))
	for _, m := range movements {
		lines = append(lines, MovementReceiptLine(m))
	}
	return "✅ Movimiento registrado\n" + strings.Join(lines, "\n")
}

// MovementReceiptLine formats one movement for a receipt/confirmation
// message: icon, category › subcategory, amount, currency, description,
// and date — enough to tell movements apart at a glance when several
// look similar. Category/subcategory come from Movement.Subcategory
// (populated at construction time or via Preload for DB-fetched candidates).
func MovementReceiptLine(m movement.Movement) string {
	category, sub := "", ""
	if m.Subcategory != nil {
		category, sub = m.Subcategory.Category, m.Subcategory.Subcategory
	}
	desc := ""
	if m.Description != nil {
		desc = *m.Description
	}
	amount := currency.FormatMoney(m.Amount.Abs(), m.Currency)
	if m.Account != nil && m.Account.Name != "" {
		return fmt.Sprintf("%s %s › %s — %s · %s · %s (%s)",
			movement.IconForType(m.Type), category, sub, amount, desc, m.Account.Name, movement.RelativeDate(m.Date))
	}
	return fmt.Sprintf("%s %s › %s — %s · %s (%s)",
		movement.IconForType(m.Type), category, sub, amount, desc, movement.RelativeDate(m.Date))
}

// MsgInsufficientFunds es el prompt del gate de saldo insuficiente: nombra qué
// cuenta quedaría negativa y cuánto le faltaría.
func MsgInsufficientFunds(short []movement.AccountShortfall) string {
	s := short[0]
	c := currency.Currency(s.Currency)
	// FormatMoney sobre el valor SIN Abs: el signo es parte de lo que se avisa.
	return fmt.Sprintf("⚠️ Ojo: %s quedaría en %s (te faltan %s). ¿Cómo lo registro?",
		s.Name, currency.FormatMoney(s.After, c), currency.FormatMoney(s.After.Abs(), c))
}
