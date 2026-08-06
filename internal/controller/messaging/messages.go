package messaging

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
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
	msgInvalidChoice  = "Esa opción no está. Tocá un botón de abajo 👇"
	msgCouldNotLoad   = "No pude traer tus datos ahora. Probá en un momento."
	msgSomethingBroke = "Se me complicó algo de mi lado, no es por vos. Probá de nuevo."

	// Ack de la cola de pending jobs (429 terminal de Groq). Nunca silencioso:
	// ackShortWaitThreshold decide cuál de las dos rinde (pending_jobs.go).
	msgAckShortWait   = "Dame un segundo, ya te lo cargo 🙌"
	msgAckLongWaitFmt = "Estoy sin cupo por ~%d min 🙏 lo cargo apenas se libere y te aviso."

	// msgQueuedBehindPending: distinto del ack del 429 (no repetir), plural implica
	// que ambos van juntos; sin jerga de cola/pendiente.
	msgQueuedBehindPending = "Ese también, ya te los cargo 🙌"

	// msgPartialSuccessAfterWrite: el turno ya insertó y después se quedó sin
	// cupo. NO se encola — reintentarlo duplicaría la plata ya registrada — así
	// que el mensaje tiene que dejar claras las dos mitades: lo que entró está
	// guardado, y lo que falte lo tiene que volver a mandar él.
	msgPartialSuccessAfterWrite = "Registré lo que me pediste, pero me quedé sin margen para el resto. Mandame de nuevo lo que falte."

	msgHelp = "Conmigo es fácil, me hablás normal:\n\n" +
		"📝 Anotar: «gasté 500 en el súper», «me pagaron 10 mil»\n" +
		"✏️ Corregir: «el súper eran 600»\n" +
		"🗑️ Borrar: «borrá el último gasto»\n" +
		"🔄 Transferir: «pasé 50 mil del banco a MP»\n" +
		"❓ Preguntar: «¿cuánto gasté esta semana?»\n" +
		"🗂️ Categorías: «creá una categoría para mascotas», «sacá la que repetí»\n" +
		"🏦 Cuentas y recordatorios: pedímelos cuando quieras."

	// MsgAccountReset is exported so the admin reset endpoint
	// (controller/admin) can send it before re-firing onboarding — the
	// admin package can't reach unexported messaging strings.
	MsgAccountReset = "🔄 Reseteamos tu cuenta. Arrancamos de nuevo:"

	msgAmountUnclear     = "No entendí el monto 🤔 ¿Lo reescribís?"
	msgCurrencyMismatch  = "Esa cuenta es de otra moneda. Reescribí el movimiento."
	msgNoAccountCurrency = "No tenés una cuenta en esa moneda. Creá una primero."
	msgMovementMalformed = "No pude armar ese movimiento. Reescribilo, porfa."

	msgQueryFailed = "No pude resolver esa consulta ahora. Probá reformularla o intentá de nuevo en un momento."

	msgAskReminderWindow       = "¿En qué franja querés que te recuerde cargar los gastos? (solo te aviso los días que no anotaste nada)"
	msgAskReminderCustomWindow = "Decime el rango en horario de 24 hs, por ejemplo: 20 a 21"
	msgInvalidReminderWindow   = "No entendí el horario. Escribilo como \"20 a 21\" (en 24 hs, de menor a mayor)."
	msgReminderDisabled        = "Dale, no te jodo más con eso 👍 Si querés que vuelva, avisame cuando quieras."
	msgReminderCancelled       = "Listo, dejé todo como estaba 👌"
	msgReminderAllOff          = "🔕 Listo, apagué todas las notificaciones: ni recordatorio diario ni resumen semanal. Cuando quieras algo de vuelta, escribime \"notificaciones\"."
	msgReminderHubExit         = "Listo 👌 Dejé todo como estaba."

	msgAskWeeklySummary = "¿Querés que te mande un resumen de tu semana todos los lunes? 📊"
	msgWeeklySummaryOn  = "📊 Listo, te mando el resumen todos los lunes."
	msgWeeklySummaryOff = "📊 Ok, no te mando el resumen semanal."

	msgSubcategorySetupFinished = "Listo, tu subcategoría está guardada ✅ " +
		"Mandame \"quiero crear otra categoría\" cuando quieras agregar más."

	msgNotUnderstood = "No te entendí 🤔 Probá de nuevo."
)

// msgCouldNotSave nombra qué no quedó guardado, para que el usuario sepa que
// su acción no se registró. cosa: "tu movimiento", "tu cuenta", "el cambio", etc.
func msgCouldNotSave(cosa string) string {
	return "No pude guardar " + cosa + ". No se guardó nada, probá de nuevo."
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

// msgCouldNotDelete es el gemelo de msgCouldNotSave para borrados: el reaseguro
// es inverso — la cosa sigue existiendo, no desapareció a medias.
func msgCouldNotDelete(cosa string) string {
	return "No pude borrar " + cosa + ". Sigue ahí, probá de nuevo."
}

// msgReminderSet builds the set/edit receipt. startMin/endMin are minutes
// since midnight; shown as whole hours.
func msgReminderSet(startMin, endMin int) string {
	return fmt.Sprintf("Listo 🙌 Te recuerdo cargar gastos entre las %d y las %d, solo los días que no hayas anotado nada.", startMin/60, endMin/60)
}

// gapPosition renders "(N de M)" for the ask-category/subcategory/account
// prompts below — idx is 0-based, total is len(rows). Shared literally by
// all three: same semantics, same format, no reason to repeat the Sprintf.
func gapPosition(idx, total int) string {
	return fmt.Sprintf(" (%d de %d)", idx+1, total)
}

func msgAskCategory(data conversation.Data) string {
	rows := decodeMovementRows(data)
	idx, _ := strconv.Atoi(decodeStringSlice(data, keyPendingCategoryGaps)[0])
	return "¿A qué categoría pertenece " + movementGapDescriptor(rows[idx]) + "?" + gapPosition(idx, len(rows))
}

func msgAskSubcategory(data conversation.Data) string {
	rows := decodeMovementRows(data)
	idx, _ := strconv.Atoi(stringOrEmpty(data[keyGapActiveRow]))
	return "¿Y la subcategoría de " + movementGapDescriptor(rows[idx]) + ", dentro de " + rows[idx].Category + "?" + gapPosition(idx, len(rows))
}

func msgAskAccount(data conversation.Data) string {
	rows := decodeMovementRows(data)
	idx, _ := strconv.Atoi(decodeStringSlice(data, keyPendingAccountGaps)[0])
	return "¿A qué cuenta corresponde " + movementGapDescriptor(rows[idx]) + "?" + gapPosition(idx, len(rows))
}

// Los tres mensajes del alta lazy-create nombran la MONEDA, porque el default
// de cuenta es por moneda. Sin eso, alguien que ya tiene una cuenta en pesos y
// carga un gasto en dólares lee "tu cuenta principal" y entiende que le pisamos
// la que ya tenía. cur vacío (no debería pasar) cae en la redacción vieja.
//
// Se nombra con Label() —"pesos", "dólares"— y nunca con el código ISO.

func msgAskFirstAccountName(cur string) string {
	if cur == "" {
		return "¿De dónde salió? Decime el nombre de la cuenta — ej: Galicia, Mercado Pago, efectivo."
	}
	return "¿De dónde salieron esos " + currency.Currency(cur).Label() + "? Decime el nombre de la cuenta — ej: Galicia, Mercado Pago, efectivo."
}

func msgAskFirstAccountBalance(name, cur string) string {
	if cur == "" {
		return "¿Cuánto saldo tenés en " + name + " ahora? Poné el saldo que ves en tu cuenta (ej: 50000) — o mandá \"después\"."
	}
	return "¿Cuánto saldo tenés en " + name + " ahora, en " + currency.Currency(cur).Label() + "? Poné el saldo que ves en tu cuenta — o mandá \"después\"."
}

// msgFirstAccountDefault nombra las monedas de las cuentas recién creadas. Son
// varias cuando un mismo mensaje trae filas en dos monedas: createFirstAccount
// crea UNA CUENTA POR MONEDA, todas con el nombre que dio el usuario.
func msgFirstAccountDefault(name string, currencies []string) string {
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

const msgInviteMoreAccounts = "Podés tener más cuentas (inversiones, dólares, lo que sea). Decime \"creá una cuenta\" cuando quieras."

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
	amount := currency.FormatMoney(m.Amount.Abs(), m.Currency)
	if m.Account != nil && m.Account.Name != "" {
		return fmt.Sprintf("%s %s › %s — %s · %s · %s (%s)",
			movement.IconForType(m.Type), category, sub, amount, desc, m.Account.Name, relativeDate(m.Date))
	}
	return fmt.Sprintf("%s %s › %s — %s · %s (%s)",
		movement.IconForType(m.Type), category, sub, amount, desc, relativeDate(m.Date))
}

// displayAmount renders a movement amount for any audience outside storage —
// the user and the LLM (as an UPDATE/DELETE candidate). The stored sign is
// internal; everyone sees the magnitude, direction comes from the type.
//
// Sin formato de miles: lo consume también el LLM (como candidato de
// UPDATE/DELETE) y ahí un "$3.000" es peor que un "3000" — para el usuario está
// rowMoney/currency.FormatMoney.
func displayAmount(d decimal.Decimal) string {
	return d.Abs().String()
}

// rowMoney formatea el monto de una fila para mostrárselo al usuario. La fila
// carga el monto como string (viaja por JSONB hacia conversation_states), así
// que hay que reparsearlo. Si no parsea se muestra crudo: un monto raro no debe
// romper el mensaje entero.
func rowMoney(r movementRow) string {
	amt, err := parseARAmount(r.Amount)
	if err != nil {
		return r.Amount + " " + currency.Currency(r.Currency).Label()
	}
	return currency.FormatMoney(amt.Abs(), currency.Currency(r.Currency))
}

// rowDate rinde la fecha de una fila en relativo ("hoy"/"ayer"/"04/07").
func rowDate(r movementRow) string {
	d, err := time.Parse("2006-01-02", r.Date)
	if err != nil {
		return r.Date
	}
	return relativeDate(d)
}

func msgPickUpdateCandidate(data conversation.Data) string {
	return "Encontré varios movimientos parecidos. ¿Cuál es?"
}

func msgConfirmUpdateDiff(data conversation.Data) string {
	before := decodeMovementRows(conversation.Data{keyMovements: data[keyBeforeMovements]})

	// regalo/gratis total: the correction zeroes the movement, so it's a
	// deletion — show what will be removed, not a "corregiría a 0" diff.
	if flag(data, keyDeleteInstead) {
		lines := []string{"🗑️ Quedó gratis, así que lo voy a borrar:"}
		for _, b := range before {
			lines = append(lines, fmt.Sprintf("%s %s › %s — %s · %s (%s)",
				iconOrDefault(b.Icon), b.Category, b.Subcategory, rowMoney(b), b.Description, rowDate(b)))
		}
		return strings.Join(lines, "\n") + "\n\n¿Confirmás?"
	}

	after := decodeMovementRows(data)

	lines := []string{"✏️ Se corregiría así:"}
	for i, a := range after {
		var b movementRow
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

const (
	msgUpdateApplied   = "✅ Corregido."
	msgUpdateDeleted   = "🗑️ Listo, lo borré (quedó gratis)."
	msgUpdateCancelled = "Cancelado, no cambié nada."

	// msgAgentActionDiscarded sale cuando se agota el presupuesto de preguntas.
	// NOMBRA lo que se cayó a propósito: tirar algo en silencio es la falla que
	// todo el parking existe para evitar.
	msgAgentActionDiscardedTemplate = "No terminé de entender %s, así que lo dejo sin hacer.\n\nSi querés, escribímelo de nuevo con un poco más de detalle."
	// Shown only when the window (today, or the mentioned day) has no
	// movements at all — the fallback picker covers every other case.
	msgNoCandidatesFound = "No tengo movimientos de ese día para tocar. ¿De qué fecha era?"

	// msgStillCannotCorrect va cuando YA le preguntamos qué cambiar y con la
	// respuesta tampoco sale una corrección. Volver a preguntar lo mismo sería
	// hacerlo girar; se corta nombrando el formato que sí funciona.
	msgStillCannotCorrect = "Sigo sin darme cuenta qué cambiarle. Probá diciéndomelo derecho — ej: «el café fueron 2000»."
)

// msgAskWhatToChange se usa cuando el movimiento SÍ se encontró pero el mensaje
// no dice qué cambiarle ("el café estaba mal"). Antes acá iba un "no me quedó
// claro, decímelo de nuevo" que era un callejón sin salida: el usuario había
// nombrado bien el movimiento y se quedaba sin nada.
//
// Pide el VALOR NUEVO, no el campo. La primera versión listaba "(el monto, la
// categoría, la fecha…)" y se leía como un menú: en la prueba real el usuario
// contestó "El monto" — nombró el campo, que es exactamente lo que no sirve.
// ResolveUpdate necesita con qué reemplazar, así que los ejemplos son
// respuestas COMPLETAS, no nombres de campo.
//
// Arranca por el monto porque es lo que se corrige casi siempre; el resto entra
// igual por el mismo texto libre.
func msgAskWhatToChange(rows []movementRow) string {
	const ask = "¿Cuánto era? Escribime el monto — o tocá abajo si lo que está mal es otra cosa."
	if len(rows) == 0 {
		return ask
	}
	return "Encontré " + movementGapDescriptor(rows[0]) + ". " + ask
}

// changeFieldOptions son los botones de la pregunta de qué cambiar. NO incluyen
// el monto a propósito: ese se escribe derecho y así el caso común —que es el
// monto— se resuelve en un paso. Los otros tres encadenan una segunda pregunta,
// porque tocar "la categoría" dice el campo pero no el valor.
//
// Sin emojis: el texto del botón se concatena al pedido que va a ResolveUpdate,
// y un emoji ahí es ruido para el modelo.
func changeFieldOptions() []string {
	return []string{"La categoría", "La fecha", "La cuenta"}
}

// msgAskChangeValue es la segunda vuelta: ya sabemos QUÉ campo, falta el valor.
const msgAskChangeValue = "Dale. ¿Y cuál es el valor nuevo?"

func msgPickDeleteCandidate(data conversation.Data) string {
	return "Encontré varios movimientos parecidos. ¿Cuál querés borrar?"
}

func msgConfirmDelete(data conversation.Data) string {
	idx, _ := strconv.Atoi(stringOrEmpty(data[keyResolvedIndex]))
	candidates := decodeCandidateGroups(data)
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

const (
	msgAskRewrite      = "✍️ Dale, mandalo de nuevo con más detalle (monto, categoría, y si es un movimiento nuevo)."
	msgCreateCancelled = "🚫 Cancelado, no registré nada."
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
	name := stringOrEmpty(data[keyAccountName])
	cur := stringOrEmpty(data[keyAccountCurrency])
	balance := stringOrEmpty(data[keyAccountBalance])
	return "Confirmá la cuenta nueva:\n\n" +
		"📛 Nombre: " + name + "\n" +
		"💱 Moneda: " + currency.Currency(cur).Label() + "\n" +
		"💰 Saldo inicial: " + balance + "\n\n" +
		"¿Confirmamos?"
}

func msgAccountCreateSuccess(name, cur, balance string) string {
	return "✅ Cuenta \"" + name + "\" creada en " + currency.Currency(cur).Label() + " con saldo inicial " + balance + "."
}

// --- ACCOUNT_MANAGE ---
// Voz: qué necesito · ejemplo · qué hago con eso. Los saldos van por
// currency.FormatMoney, que conserva el signo — un saldo puede ser negativo y
// el usuario tiene que verlo tal cual (NO displayAmount, que hace Abs(): esa es
// para magnitudes de movimientos). Y como FormatMoney ya trae el símbolo
// ("$" / "US$"), el código de moneda no se repite al lado del número.

func msgAccountManagePick(data conversation.Data) string {
	return "¿De cuál de tus cuentas me hablás? Elegila acá abajo y te muestro qué se puede hacer."
}

func msgAccountManageMenu(name, cur string, balance decimal.Decimal) string {
	c := currency.Currency(cur)
	return fmt.Sprintf("Cuenta: %s (%s) — saldo actual %s.\n¿Qué querés hacer con ella?",
		name, c.Label(), currency.FormatMoney(balance, c))
}

func msgAskAccountNewName(current string) string {
	return fmt.Sprintf("✏️ Decime el nombre nuevo para %s.\nEs el nombre con el que la vas a nombrar en tus mensajes.\n\nEj: FCI", current)
}

func msgConfirmAccountRename(oldName, newName string) string {
	return fmt.Sprintf("Renombro %s → %s. ¿Confirmás?", oldName, newName)
}

func msgAskAccountNewTotal(name string) string {
	return fmt.Sprintf("💰 Decime cuánto tenés en total hoy en %s.\nPoné el número que ves en tu banco o app.\nYo calculo la diferencia con lo registrado y la ajusto.\n\nEj: 52000", name)
}

// El renglón del medio no es decoración: el ajuste se registra como movimiento,
// y sin decirlo el usuario ve aparecer un "ingreso" que nunca cobró. Pasó de
// verdad — alguien ajustó una cuenta con CEDEARs y 24 minutos después pidió
// borrar el ajuste. Se dice ANTES de confirmar, que es cuando todavía es
// información y no una sorpresa.
func msgConfirmAccountAdjust(name, cur string, current, newTotal decimal.Decimal) string {
	delta := newTotal.Sub(current)
	sign := "+"
	if delta.IsNegative() {
		sign = "-"
	}
	c := currency.Currency(cur)
	return fmt.Sprintf("%s: %s → %s (ajuste %s%s)\n\nCorrige el saldo, no cuenta como gasto ni ingreso.\n¿Confirmás?",
		name, currency.FormatMoney(current, c), currency.FormatMoney(newTotal, c), sign, currency.FormatMoney(delta.Abs(), c))
}

func msgConfirmAccountDefault(name, cur string) string {
	label := currency.Currency(cur).Label()
	return fmt.Sprintf("⭐ ¿%s pasa a ser tu cuenta en %s por defecto? Los movimientos en %s sin cuenta aclarada van a ir ahí.", name, label, label)
}

// msgFlowCancelled: "cancelaste, no escribí nada" — no es específico de
// cuentas ni categorías, cualquier flujo de gestión que se cancela lo usa.
const msgFlowCancelled = "Listo, no toqué nada."
const msgAccountManageNoChange = "Ya tenías ese saldo, no cambié nada."

// msgAskSubcategoryDescription is deliberately short and concrete: the
// answer feeds orchestrator.TaxonomyEntry.Description, Call 2 CREATE's
// classification hint, so it must tell the LLM when/what this
// subcategory refers to — not just be a decorative label.
func msgAskSubcategoryDescription(sub string) string {
	return "En una frase: ¿cuándo se usa \"" + sub + "\"? (ej: \"gastos de comida y snacks en la calle\")"
}

// msgCategoryMatchOffer is shown when the LLM matched a CREATE_CATEGORY
// request to an existing taxonomy entry — offer to reuse it instead of
// creating a duplicate.
func msgCategoryMatchOffer(data conversation.Data) string {
	icon := stringOrEmpty(data[keyCategoryIcon])
	if icon == "" {
		icon = "📂"
	}
	out := "Ya tenés una parecida: " + icon + " " + stringOrEmpty(data[keyCategory]) + " › " + stringOrEmpty(data[keySubcategory])
	if desc := stringOrEmpty(data[keySubcategoryDescription]); desc != "" {
		out += "\n📝 " + desc
	}
	return out + "\n\n¿Te sirve o creás una distinta?"
}

// msgCategoryProposalConfirm shows the LLM's complete proposal for a new
// subcategory as one confirmation.
func msgCategoryProposalConfirm(data conversation.Data) string {
	icon := stringOrEmpty(data[keyCategoryIcon])
	if icon == "" {
		icon = "📂"
	}
	line := icon + " " + stringOrEmpty(data[keyCategory]) + " › " + stringOrEmpty(data[keySubcategory])
	if flag(data, keyCategoryIsNew) {
		line += " (categoría nueva)"
	}
	return "Te propongo:\n" + line + "\n📝 " + stringOrEmpty(data[keySubcategoryDescription]) + "\n\n¿La creo?"
}

const msgCategoryMatchUse = "Listo ✅ — registrá el gasto nombrándolo y cae ahí solo (ej: \"gasté 5000 en un regalo\")."

const msgResumeCancelled = "Cancelado ✅ — arrancá de nuevo cuando quieras."

func msgInsufficientFunds(short []movement.AccountShortfall) string {
	s := short[0]
	c := currency.Currency(s.Currency)
	// FormatMoney sobre el valor SIN Abs: el signo es parte de lo que se avisa.
	return fmt.Sprintf("⚠️ Ojo: %s quedaría en %s (te faltan %s). ¿Cómo lo registro?",
		s.Name, currency.FormatMoney(s.After, c), currency.FormatMoney(s.After.Abs(), c))
}

const msgLogMissingFirst = "Dale, registrá primero lo que falta y volvé a mandarme esto."

// FlowResumeLabel gives the resume gate (conversation.Engine) a short,
// per-flow description of what the user was doing, for its "¿retomamos o
// cancelamos?" prompt. One entry per registered flow; an unregistered or
// unrecognized name falls back to a generic phrase.
func FlowResumeLabel(flowName string) string {
	switch flowName {
	case movementCreateFlowName:
		return "estabas registrando un movimiento"
	case movementUpdatePickFlowName, movementUpdateConfirmFlowName:
		return "estabas corrigiendo un movimiento"
	case movementDeleteFlowName:
		return "estabas borrando un movimiento"
	case accountCreateFlowName:
		return "estabas creando una cuenta"
	case subcategorySetupFlowName:
		return "estabas creando una subcategoría"
	case categoryMatchOfferFlowName, categoryProposalConfirmFlowName:
		return "estabas creando una categoría"
	case movementNegativeConfirmFlowName:
		return "estabas confirmando un movimiento"
	case askUserFlowName:
		return "algo que te pregunté"
	default:
		return "una conversación anterior"
	}
}
