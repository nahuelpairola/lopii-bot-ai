// Package query es el loop de QUERY (consultas read-only al asistente). Vive
// en su propio paquete (extraído de messaging en la costura de la etapa 6) y
// NO sabe nada de Telegram-webhook ni del controller.
//
// Lo que el loop necesita del mundo exterior es la interfaz services, que el
// borde (controller/messaging) implementa con puentes de una línea en
// query_services.go. Este paquete nunca importa internal/controller/messaging.
package query

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/go-telegram/bot"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/agent"
	"lopiibot.com/internal/chathistory"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
)

// services es la vista del loop de QUERY sobre el controller de messaging.
// La implementa *controller estructuralmente desde query_services.go — este
// paquete nunca importa internal/controller/messaging.
type services interface {
	QueryAccountsByUserID(userID uint64) ([]account.Account, error)
	QueryCategoriesByUser(userID uint64) ([]subcategory.Subcategory, error)
	QueryIconForCategory(userID uint64, category string) string
	QueryListMovements(q movement.MovementQuery, limit int) ([]movement.Movement, error)
	QuerySumMovements(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error)
	QueryBalanceForAccount(accountID uint64) (decimal.Decimal, error)
	QueryReminderByUser(userID uint64) (*reminder.Reminder, error)
	QueryChatRecent(userID uint64) ([]chathistory.Turn, error)
	QueryChatAppend(userID uint64, question, answer string) error
	QuerySendText(ctx context.Context, b *bot.Bot, chatID int64, text string)
	AnswerQuery(ctx context.Context, systemPrompt, userText string, history []orchestrator.QueryTurn, tools []orchestrator.AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error)
}

// Tools are the read-only tools the QUERY loop composes. Invariants
// live in the executor (Go), not here — the model only picks tools + ranges.
// Optional params are declared nullable (`["string","null"]`) — the tool-
// calling models routinely emit an explicit `null` for an argument they don't
// want to set, and Groq validates arguments against the schema server-side, so
// a plain `"string"` type 400s on that null before the executor ever runs.
// json.Unmarshal of null leaves the Go zero value, so the executor already
// treats it as "absent". Only from/to/currency are required (never null).
var Tools = []orchestrator.AgentTool{
	{
		Name:        "list_categories",
		Description: "Lista las categorías y subcategorías disponibles. Sin filtro devuelve el listado completo (categoría | subcategoría). Pasá category para acotarla a una sola categoría: ahí además viene la descripción de cuándo usar cada subcategoría. Usala cuando el usuario pregunta qué categorías existen o para qué sirve una.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"category": {"type": ["string", "null"], "description": "opcional: filtrar a una sola categoría"}
			}
		}`),
	},
	{
		Name:        "sum_movements",
		Description: "Suma montos de movimientos en un rango de fechas, opcionalmente agrupado. Devuelve montos en positivo. Excluye transferencias entre cuentas propias salvo que se pida type=transfer.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"from": {"type": "string", "description": "fecha desde YYYY-MM-DD"},
				"to": {"type": "string", "description": "fecha hasta YYYY-MM-DD"},
				"currency": {"type": "string", "enum": ["ARS", "USD"]},
				"group_by": {"type": ["string", "null"], "enum": ["none", "category", "subcategory", "type", "month", "day", "account", null]},
				"type": {"type": ["string", "null"], "enum": ["expense", "income", "transfer", null], "description": "opcional; sin esto se excluyen las transferencias"},
				"account": {"type": ["string", "null"], "description": "opcional: nombre de una cuenta del usuario"},
				"search": {"type": ["string", "null"], "description": "opcional: texto a buscar. Matchea contra el nombre de la categoría, el de la subcategoría y la descripción del movimiento, sin distinguir mayúsculas ni acentos. Ej: \"alimentacion\", \"netflix\", \"lote\"."}
			},
			"required": ["from", "to", "currency"]
		}`),
	},
	{
		Name:        "list_movements",
		Description: "Lista movimientos individuales (los más recientes primero) en un rango de fechas, con filtros opcionales. Montos en positivo.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"from": {"type": "string", "description": "fecha desde YYYY-MM-DD"},
				"to": {"type": "string", "description": "fecha hasta YYYY-MM-DD"},
				"currency": {"type": "string", "enum": ["ARS", "USD"]},
				"type": {"type": ["string", "null"], "enum": ["expense", "income", "transfer", null]},
				"account": {"type": ["string", "null"]},
				"search": {"type": ["string", "null"], "description": "opcional: texto a buscar. Matchea contra el nombre de la categoría, el de la subcategoría y la descripción del movimiento, sin distinguir mayúsculas ni acentos. Ej: \"alimentacion\", \"netflix\", \"lote\"."},
				"limit": {"type": ["integer", "null"], "description": "máximo de filas (default 20, tope 50)"}
			},
			"required": ["from", "to", "currency"]
		}`),
	},
	{
		Name:        "account_balance",
		Description: "Saldo actual de una cuenta o de todas las cuentas del usuario. El saldo es la suma de sus movimientos. Nunca mezcla ARS y USD.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"account": {"type": ["string", "null"], "description": "opcional: nombre de una cuenta; sin esto, todas"}
			}
		}`),
	},
	{
		Name:        "get_reminder",
		Description: "Devuelve el recordatorio diario de carga de gastos del usuario: si está activo o apagado y en qué franja horaria avisa. Usala cuando el usuario pregunta por su recordatorio (\"¿a qué hora me recordás?\", \"¿tengo recordatorio activo?\").",
		Parameters:  json.RawMessage(`{"type": "object", "properties": {}}`),
	},
}

// queryToolArgs is the union of every tool's argument shape — one struct
// keeps the executor's json.Unmarshal simple (unused fields stay zero).
type queryToolArgs struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Currency string `json:"currency"`
	GroupBy  string `json:"group_by"`
	Type     string `json:"type"`
	Account  string `json:"account"`
	Search   string `json:"search"`
	Limit    int    `json:"limit"`
	// Category ya NO filtra movimientos: es el parámetro de list_categories, que
	// acota el listado de TAXONOMÍA a una sola categoría. Las dos tools de
	// movimientos filtran con Search.
	Category string `json:"category"`
}

// Los cuatro desenlaces de una consulta que no devolvió filas. Son lo que ve el
// MODELO, no el usuario, y por eso viven acá y no en messages.go.
//
// Son cuatro y no uno porque describen HECHOS DISTINTOS, y hasta el 2026-08-14
// los cuatro salían por el mismo string ambiguo: el modelo tenía que elegir cuál
// creer, y el 2026-08-13 eligió mal —contestó "No tenés registros de gastos en la
// categoría Lote. El total gastado es $0 ARS" sobre $30.343,74 reales—.
//
// NINGUNO puede nombrar una herramienta. La narración forzada corre con
// tool_choice:"none" y sin schemas, y ahí el modelo imita todo lo que se parezca
// a una tool: la versión que decía "verificá con list_categories" hizo que
// gpt-oss-20b devolviera 400 y que gpt-oss-120b le imprimiera al usuario
// {"tool": "list_categories", "params": {}}. Lo fija TestQueryMessages_NameNoTool.
const (
	// Sin search, un cero es un cero honesto: no hubo movimientos en ese rango.
	// No hay filtro de texto que pueda no haber matcheado.
	msgQueryNoRowsInRange = "sin movimientos en ese rango."

	// El término existe en los datos del usuario, pero no en el rango pedido.
	// Es una AUSENCIA VERIFICADA: acá el modelo sí puede decir que no gastó.
	msgSearchOutOfRangeFmt = "sin movimientos con «%s» entre %s y %s. " + msgOutOfRangeMark

	// msgOutOfRangeMark identifica el mensaje anterior a la vuelta, para reponer
	// el rango que el modelo casi siempre tira. Const separada por lo mismo que
	// msgOnlyInternalMark: se usa para armarlo y para reconocerlo.
	msgOutOfRangeMark = "Sí hay con ese texto en otras fechas."

	// msgConsultedRangeFmt es la nota al pie que la app le agrega a una respuesta
	// que salió vacía. No la escribe el modelo: es la ventana que la app CONSULTÓ
	// de verdad, y es lo único que delata un año mal resuelto.
	msgConsultedRangeFmt = "(consulté entre %s y %s)"

	// El término sólo matchea movimientos de categorías reservadas, que apply()
	// esconde de todo total de gastos e ingresos. Sin este mensaje la app diría
	// que "transferencia" no existe, sobre 12 movimientos reales.
	msgSearchOnlyInternalFmt = "«%s» " + msgOnlyInternalMark + " —transferencias entre tus cuentas, saldos iniciales, ajustes—, que no entran en los totales de gastos e ingresos."

	// msgOnlyInternalMark es la parte del mensaje anterior que lo identifica, y
	// existe como const separada porque se usa DOS veces: para armarlo y para
	// reconocerlo a la vuelta en reinstateAppVerdict.
	msgOnlyInternalMark = "sólo aparece en movimientos internos"

	// El término no matchea NADA. Es lo único que habilita decir que no existe,
	// y va como ERROR para que el modelo lo pueda corregir en la ronda siguiente:
	// AnswerQuery reinyecta los errores del ejecutor en vez de abortar.
	msgSearchNotFoundFmt = "no encontré nada que diga «%s»: no es una categoría, ni una subcategoría, ni aparece en ninguna descripción."
)

// searchProbeFrom/To es el rango "todo el historial" de las sondas. Fechas fijas
// y absurdamente anchas a propósito: la sonda contesta "¿existe este término en
// algún lado?", y una ventana relativa a hoy haría que la respuesta cambiara
// sola con el paso del tiempo.
var (
	searchProbeFrom = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	searchProbeTo   = time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
)

// describeEmptyResult decide qué decir cuando la consulta no devolvió filas.
// Devuelve (mensaje, nil) o ("", error) — el error es "el término no existe".
//
// Las sondas corren SÓLO acá, o sea sólo cuando el resultado ya vino vacío, son
// LIMIT 1, y cuestan cero tokens: son consultas a Postgres, no llamadas a Groq.
func describeEmptyResult(svc services, q movement.MovementQuery, args queryToolArgs) (string, error) {
	if q.Search == nil {
		return msgQueryNoRowsInRange, nil
	}
	term := *q.Search

	wide := q
	wide.From, wide.To = searchProbeFrom, searchProbeTo
	if rows, err := svc.QueryListMovements(wide, 1); err == nil && len(rows) > 0 {
		return fmt.Sprintf(msgSearchOutOfRangeFmt, term, args.From, args.To), nil
	}

	// Sonda 2: las reservadas. apply() las excluye siempre salvo que se pidan, y
	// el ejecutor de query nunca las pide, así que la sonda 1 las esconde igual
	// que la consulta real. Sin esto, "transferencia" y "saldo inicial" —12 y 4
	// movimientos reales en la base local— se declararían inexistentes.
	wide.OnlyReserved = true
	if rows, err := svc.QueryListMovements(wide, 1); err == nil && len(rows) > 0 {
		return fmt.Sprintf(msgSearchOnlyInternalFmt, term), nil
	}

	// Segunda pasada de la sonda 2, forzando el tipo. Las reservadas que MÁS
	// importan —Sistema | Transferencia y los saldos iniciales— son todas
	// type=transfer, y con Type nil apply agrega `type <> transfer`: las esconde
	// justo cuando hacen falta. Medido el 2026-08-14 contra la base: la sonda con
	// type=transfer encuentra 12 filas y la misma sonda con Type nil encuentra 0,
	// y esas 12 se declaraban inexistentes.
	//
	// Va como segunda consulta y no reemplazando a la de arriba porque las otras
	// reservadas —Ajuste de saldo, Rendimiento inversión— NO son transferencias:
	// una sola pasada, con o sin tipo, siempre deja afuera la mitad.
	if wide.Type == nil {
		t := constants.Transfer
		wide.Type = &t
		if rows, err := svc.QueryListMovements(wide, 1); err == nil && len(rows) > 0 {
			return fmt.Sprintf(msgSearchOnlyInternalFmt, term), nil
		}
	}

	return "", fmt.Errorf(msgSearchNotFoundFmt, term)
}

// reinstateAppVerdict devuelve la respuesta con el veredicto de la app pegado atrás,
// si el modelo lo perdió por el camino.
//
// Existe porque el 2026-08-14, en producción, el modelo INVIRTIÓ el veredicto: el
// ejecutor le entregó "«transferencia» sólo aparece en movimientos internos…" —con 12
// filas reales detrás, verificadas por la sonda— y el usuario leyó "No se encontraron
// movimientos que digan transferencia en agosto de 2026". No es un matiz perdido: es la
// afirmación contraria a la que hizo la app.
//
// Este es el único mensaje que se reinstala, y no todos, porque es el único donde el
// modelo puede leer un resultado vacío y concluir lo opuesto a lo que dice el texto. Los
// otros tres describen ausencias de verdad: si los aplasta, empobrece la respuesta pero
// no la vuelve falsa.
//
// Se pega SÓLO si la respuesta no habla ya de movimientos internos, para no repetir lo
// que el modelo sí supo decir.
// emptyResultNamesARange dice si un resultado del ejecutor es uno de los vacíos
// cuyo rango vale la pena reponer.
//
// Son dos de los cuatro desenlaces, los que hablan de una VENTANA: la app miró un
// período concreto y no encontró nada, así que el período es el dato sospechoso.
// Los otros dos son hechos sobre el TÉRMINO —"sólo aparece en movimientos
// internos", "no encontré nada que diga X"—, verdaderos en cualquier rango, y
// reponerles una ventana sólo agregaría ruido.
func emptyResultNamesARange(out string) bool {
	return strings.Contains(out, msgQueryNoRowsInRange) || strings.Contains(out, msgOutOfRangeMark)
}

// appendConsultedRange le pega a la respuesta la ventana que la app consultó,
// cuando la consulta volvió vacía.
//
// Una consulta que sale vacía porque el modelo resolvió mal el año es INVISIBLE:
// el ejecutor dice "sin movimientos con «Supermercado» entre 2024-08-01 y
// 2024-08-31", el modelo redacta "no gastaste en Supermercado", y el rango —el
// único dato que delata el error— no llega nunca al usuario. Esto no previene la
// resolución equivocada; la hace visible en el acto.
//
// Va como nota al pie propia de la app y no reponiendo el mensaje crudo del
// ejecutor, por dos razones: el mensaje crudo trae las fechas en ISO, que el
// prompt le prohíbe mostrar al modelo y por lo tanto la app tampoco puede colar
// por atrás; y una línea corta no compite con la respuesta que el usuario pidió.
func appendConsultedRange(answer, from, to string) string {
	if from == "" || to == "" || strings.Contains(answer, from) {
		return answer
	}
	return strings.TrimSpace(answer) + "\n" + fmt.Sprintf(msgConsultedRangeFmt, from, to)
}

// friendlyDate pasa una fecha ISO al formato argentino. Lo que no parsea vuelve
// tal cual: el rango es informativo, y nunca vale romper una respuesta que ya
// está lista por una fecha rara.
func friendlyDate(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return t.Format("02/01/2006")
}

func reinstateAppVerdict(answer, verdict string) string {
	if verdict == "" || strings.Contains(strings.ToLower(answer), "internos") {
		return answer
	}
	return strings.TrimSpace(answer) + "\n\n" + verdict
}

// Run answers a read-only question via the agent loop. Returns
// (answered, err): answered=false significa que el loop no produjo respuesta.
//
// El fracaso NO manda copy acá — la manda el caller, a propósito. Un 429 se encola
// y se ackea (handleGroqError); si esta función mandara msgQueryFailed por su cuenta,
// el usuario leería "no pude responder" Y el ack de la cola por el mismo mensaje.
// Solo el caller sabe distinguir un 429 encolado de un fracaso de verdad.
func Run(ctx context.Context, svc services, b *bot.Bot, chatID int64, userID uint64, text string) (bool, error) {
	prompt := SystemPrompt()

	// El wrapper mira lo que DEVOLVIÓ el ejecutor, no lo que el modelo hizo con eso:
	// es la única forma de enterarse de que la app emitió un veredicto propio sin
	// cambiarle la firma a buildQueryExecutor, que usan quince tests.
	var appVerdict, rangeFrom, rangeTo string
	inner := NewExecutor(svc, userID)
	execute := func(name string, raw json.RawMessage) (string, error) {
		out, err := inner(name, raw)
		if strings.Contains(out, msgOnlyInternalMark) {
			appVerdict = out
		}
		// El rango sale de los argumentos con los que el modelo LLAMÓ, que es
		// justamente el dato en discusión: si resolvió mal el año, acá está el año
		// equivocado, y reponerlo es lo que lo vuelve visible.
		if emptyResultNamesARange(out) {
			var args queryToolArgs
			if json.Unmarshal(raw, &args) == nil {
				rangeFrom, rangeTo = friendlyDate(args.From), friendlyDate(args.To)
			}
		}
		return out, err
	}

	// Best-effort: a history load error never fails the query — run stateless.
	turns, _ := svc.QueryChatRecent(userID)
	history := make([]orchestrator.QueryTurn, len(turns))
	for i, t := range turns {
		history[i] = orchestrator.QueryTurn{Question: t.Question, Answer: t.Answer}
	}

	answer, err := svc.AnswerQuery(ctx, prompt, text, history, Tools, execute)
	if err != nil || strings.TrimSpace(answer) == "" {
		return false, err
	}
	answer = reinstateAppVerdict(answer, appVerdict)
	answer = appendConsultedRange(answer, rangeFrom, rangeTo)
	svc.QuerySendText(ctx, b, chatID, answer)
	// Best-effort append: a failure here never fails the answer the user already got.
	_ = svc.QueryChatAppend(userID, text, answer)
	return true, nil
}

func SystemPrompt() string {
	today := agent.StartOfTodayArgentina().Format("2006-01-02")
	return fmt.Sprintf(`Sos el asistente de consultas de un bot de finanzas personales argentino.
Basá TODA cifra en los datos que devuelven las herramientas — nunca inventes ni estimes un número sin respaldo de una herramienta.
Sí podés hacer aritmética SOBRE esos datos: sumar, restar, promediar o sacar tasas por día/mes. Para un promedio mensual, pedí los totales por mes (group_by=month) y dividí. Para comparar dos períodos ("cuánto más que el mes pasado"), pedí cada total y restá. Para una tasa diaria, dividí el total por la cantidad de días del rango.
Hoy es %s (zona America/Argentina/Buenos_Aires). Resolvé fechas relativas ("hoy", "ayer", "esta semana", "el mes pasado", "mayo") a rangos concretos YYYY-MM-DD antes de llamar una herramienta.
Los montos se muestran siempre en positivo. ARS y USD son mundos separados: nunca los sumes ni los conviertas; si hacen falta ambos, reportá cada uno por su lado.
Nunca hagas una pregunta de aclaración — no podés recibir la respuesta del usuario. Si la consulta es ambigua entre varias categorías o cuentas conocidas, resolvela vos: usá list_categories para ver las que aplican y respondé TODAS las interpretaciones plausibles en la misma respuesta.
Una ausencia es un hecho y necesita respaldo igual que un monto: decí que algo no tiene registros SÓLO si lo consultaste y la herramienta volvió vacía. De lo que no llegaste a consultar, decí que no lo averiguaste — nunca que no existe, que no hay, ni que dio cero.
Si la pregunta nombra varias cosas —varias categorías, varias cuentas, varios períodos—, pedí en la MISMA ronda todas las herramientas que necesites, una por cada cosa. Tenés pocas rondas: de a una no alcanza y te quedás sin averiguar parte de lo que te preguntaron.
Cuando tengas los datos, respondé en español rioplatense, claro y breve.
No uses Markdown ni caracteres decorativos: nada de *, **, _, #, ni guiones largos como separadores — Telegram los muestra crudos. Escribí texto plano, prolijo y bien organizado: líneas cortas, un ítem por línea cuando enumeres.
Montos en formato argentino: separador de miles con punto y símbolo adelante ($5.500, $1.234,56); no muestres los centavos ".00"/",00" cuando el monto es entero de pesos. Aclará la moneda (ARS/USD) cuando haga falta.
Fechas en formato amable (01/07 o "1 de julio"), nunca 2026-07-01.
Cuando una herramienta te da un emoji junto a una categoría, poné ese emoji al principio de la línea para que se lea visual.
Para preguntas sobre el recordatorio de carga de gastos (si está activo, a qué hora avisa), usá get_reminder.
Si la pregunta no se puede responder con estas herramientas, decilo con amabilidad en una línea.`, today)
}

// NewExecutor returns the execute closure the loop calls per tool call. It is
// scoped to userID and owns every invariant (user-scoping, abs amounts,
// ARS/USD separation) — the LLM can only pick tools and ranges.
func NewExecutor(svc services, userID uint64) func(string, json.RawMessage) (string, error) {
	return func(name string, raw json.RawMessage) (string, error) {
		var args queryToolArgs
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", fmt.Errorf("argumentos inválidos: %w", err)
		}
		switch name {
		case "list_categories":
			return execListCategories(svc, userID, args)
		case "sum_movements":
			return execSumMovements(svc, userID, args)
		case "list_movements":
			return execListMovements(svc, userID, args)
		case "account_balance":
			return execAccountBalance(svc, userID, args)
		case "get_reminder":
			rem, err := svc.QueryReminderByUser(userID)
			if err != nil {
				rem = nil // no row (or lookup miss) -> "no configurado"
			}
			return describeReminder(rem), nil
		default:
			return "", fmt.Errorf("herramienta desconocida: %s", name)
		}
	}
}

func execListCategories(svc services, userID uint64, args queryToolArgs) (string, error) {
	subs, err := svc.QueryCategoriesByUser(userID)
	if err != nil {
		return "", err
	}
	// Las descripciones ("cuándo usarla") SOLO viajan en la lista filtrada, y no en
	// la completa. Medido el 2026-08-10 con 66 filas: la lista entera con
	// descripciones son ~1.325 tokens y sin ellas ~500. El resultado de una tool se
	// reinyecta en CADA ronda posterior, así que esos ~825 tokens se pagan dos o tres
	// veces por consulta contra un TPM de 8.000 — y una consulta multi-entidad se
	// pasaba del techo justo por eso (ver el modelo de costo en
	// orchestrator/client_loop.go).
	//
	// Para CONTESTAR alcanza el mapeo nombre → categoría | subcategoría; las
	// descripciones existen para clasificar en CREATE, no para consultar. Cuando el
	// modelo sí las necesita para desambiguar, filtra por categoría y ahí el bloque
	// es chico y las manda completas.
	withDescriptions := args.Category != ""
	var lines []string
	for _, s := range subs {
		if args.Category != "" && s.Category != args.Category {
			continue
		}
		line := fmt.Sprintf("%s | %s", s.Category, s.Subcategory)
		// Misma regla que orchestrator.buildTaxonomyBlock: una descripción vacía
		// es deliberada (la migración de podado vacía las notas que no
		// desambiguan), no un dato faltante. Sin esta guarda la respuesta al
		// usuario sale con un separador colgante — "Alimentación | Supermercado | " —
		// en 26 de las 65 globales.
		if withDescriptions && s.Description != "" {
			line += " | " + s.Description
		}
		if s.Icon != "" {
			line = s.Icon + " " + line
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return "No hay categorías que coincidan.", nil
	}
	header := "categoría | subcategoría:\n"
	if withDescriptions {
		header = "categoría | subcategoría | cuándo usarla:\n"
	}
	return header + strings.Join(lines, "\n"), nil
}

func execSumMovements(svc services, userID uint64, args queryToolArgs) (string, error) {
	q, err := buildMovementQuery(svc, userID, args)
	if err != nil {
		return "", err
	}
	groupBy := args.GroupBy
	rows, err := svc.QuerySumMovements(q, groupBy)
	if err != nil {
		return "", err
	}
	// Un sum SIN agrupar devuelve SIEMPRE una fila —`COALESCE(SUM(ABS(amount)), 0)`
	// sin GROUP BY es una fila con cero, nunca cero filas—, así que por ese camino
	// "no encontré nada" llega disfrazado de total en cero y describeEmptyResult
	// era INALCANZABLE. Y es el camino más común de todos: "¿cuánto gasté en X?".
	//
	// Medido contra el bot el 2026-08-18: preguntar por Supermercado en junio
	// devolvía "total: 0.00 ARS" y el modelo narraba "no hay registros de gastos en
	// Supermercado" — el mismo cero mudo que los cuatro mensajes vinieron a matar,
	// vivo en la mitad del tráfico. Los tests no lo veían porque el fake devolvía
	// nil, una forma que el repo no produce jamás.
	//
	// SUM(ABS()) nunca da negativo, así que un cero sale de no haber sumado nada —o
	// de haber sumado sólo movimientos de monto cero ("me lo regalaron"), que caen
	// en las sondas y se describen como ausencia. Impreciso en ese borde, y aun así
	// mejor que el cero mudo.
	if len(rows) == 0 || (ungroupedSum(groupBy) && rows[0].Total.IsZero()) {
		return describeEmptyResult(svc, q, args)
	}
	cur := q.Currency.String()
	if ungroupedSum(groupBy) {
		return fmt.Sprintf("total: %s %s", rows[0].Total.Abs().StringFixed(2), cur), nil
	}
	// For account grouping, map account_id labels to names.
	nameByID := map[string]string{}
	if groupBy == "account" {
		accts, _ := svc.QueryAccountsByUserID(userID)
		for _, a := range accts {
			nameByID[fmt.Sprintf("%d", a.ID)] = a.Name
		}
	}
	var lines []string
	for _, r := range rows {
		label := r.Label
		if groupBy == "account" {
			if n, ok := nameByID[label]; ok {
				label = n
			}
		}
		if groupBy == "category" && label != "" {
			label = svc.QueryIconForCategory(userID, label) + " " + label
		}
		lines = append(lines, fmt.Sprintf("%s: %s %s", label, r.Total.Abs().StringFixed(2), cur))
	}
	if line, ok := groupedTotalLine(rows, groupBy, cur); ok {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

// ungroupedSum dice si el pedido no lleva agrupación. Son dos valores y no uno
// porque el schema declara "none" explícito y el modelo también puede omitir el
// campo, y las dos cosas significan lo mismo.
func ungroupedSum(groupBy string) bool {
	return groupBy == movement.GroupByNone || groupBy == groupByNoneArg
}

// groupByNoneArg es el "none" del enum del schema. movement.GroupByNone es el
// string vacío que entiende el repo; el modelo manda esta otra palabra.
const groupByNoneArg = "none"

// groupedTotalLine arma la línea de total de un agrupado, o dice que no va.
//
// Existe porque el modelo no suma: el 2026-08-14 recibió dos filas —Supermercado
// 2.031.070 y Almacén 34.000— y contestó 2.031.070, la primera. Es el mismo
// principio que ya gobierna las correcciones: la aritmética es de la app, y el
// modelo sólo cita lo que la app calculó.
//
// Dos casos NO llevan total, y los dos son por corrección, no por estética:
//
//   - group_by=type. Las filas llegan en valor absoluto (CategorySum.Total es
//     SUM(ABS(amount))), así que sumar el renglón de gastos con el de ingresos da
//     un número que no es el gasto, ni el ingreso, ni el neto. Escribirlo sería
//     peor que no escribir nada: la línea existe justamente para que el modelo la
//     cite sin revisarla.
//   - Una sola fila. El total ES la fila, y repetirlo le presenta dos hechos
//     donde hay uno.
func groupedTotalLine(rows []movement.CategorySum, groupBy, cur string) (string, bool) {
	if groupBy == "type" || len(rows) < 2 {
		return "", false
	}
	total := decimal.Zero
	for _, r := range rows {
		total = total.Add(r.Total.Abs())
	}
	return fmt.Sprintf("total (suma de las %d filas): %s %s", len(rows), total.StringFixed(2), cur), true
}

func execListMovements(svc services, userID uint64, args queryToolArgs) (string, error) {
	q, err := buildMovementQuery(svc, userID, args)
	if err != nil {
		return "", err
	}
	ms, err := svc.QueryListMovements(q, args.Limit)
	if err != nil {
		return "", err
	}
	if len(ms) == 0 {
		return describeEmptyResult(svc, q, args)
	}
	var lines []string
	for _, m := range ms {
		lines = append(lines, queryMovementLine(m))
	}
	return strings.Join(lines, "\n"), nil
}

// queryMovementLine renders one movement row for a QUERY answer. It uses
// Amount.Abs() deliberately: ListForUser returns DB rows with the stored
// SIGNED amount (an expense is negative), and the sign must never surface —
// direction is the movement type, not a minus. (movementReceiptLine renders
// the raw amount for CREATE receipts, where the draft is already positive;
// QUERY needs the explicit Abs, so it has its own line renderer.)
func queryMovementLine(m movement.Movement) string {
	cat, sub := "", ""
	if m.Subcategory != nil {
		cat, sub = m.Subcategory.Category, m.Subcategory.Subcategory
	}
	desc := ""
	if m.Description != nil {
		desc = *m.Description
	}
	return fmt.Sprintf("%s %s › %s — %s %s · %s (%s)",
		movement.IconForType(m.Type), cat, sub,
		m.Amount.Abs().StringFixed(2), m.Currency.String(), desc,
		m.Date.Format("2006-01-02"))
}

func execAccountBalance(svc services, userID uint64, args queryToolArgs) (string, error) {
	accts, err := svc.QueryAccountsByUserID(userID)
	if err != nil {
		return "", err
	}
	var lines []string
	for _, a := range accts {
		if args.Account != "" && !strings.EqualFold(a.Name, args.Account) {
			continue
		}
		bal, err := svc.QueryBalanceForAccount(uint64(a.ID))
		if err != nil {
			return "", err
		}
		lines = append(lines, fmt.Sprintf("%s (%s): %s", a.Name, a.Currency.String(), bal.StringFixed(2)))
	}
	if len(lines) == 0 {
		return "No encontré esa cuenta.", nil
	}
	return strings.Join(lines, "\n"), nil
}

// buildMovementQuery translates tool args into a movement.MovementQuery,
// resolving the optional account name to an ID and validating the currency.
func buildMovementQuery(svc services, userID uint64, args queryToolArgs) (movement.MovementQuery, error) {
	cur := currency.Currency(args.Currency)
	if cur != currency.ARS && cur != currency.USD {
		cur = currency.ARS // default per the ARS-if-unspecified convention
	}
	from, err := parseQueryDate(args.From)
	if err != nil {
		return movement.MovementQuery{}, fmt.Errorf("fecha 'from' inválida: %w", err)
	}
	to, err := parseQueryDate(args.To)
	if err != nil {
		return movement.MovementQuery{}, fmt.Errorf("fecha 'to' inválida: %w", err)
	}
	q := movement.MovementQuery{
		UserID:   userID,
		From:     from,
		To:       to,
		Currency: cur,
	}
	if args.Type != "" {
		t := args.Type
		q.Type = &t
	}
	// stripLeadingIcon SIGUE haciendo falta, y ahora sobre un solo campo. El
	// prompt le pide al modelo arrancar la línea con el emoji, list_categories
	// devuelve "🍔 Alimentación", el modelo aprende ese string y lo copia al
	// filtro. unaccent no borra emojis: sin esto, LIKE '%🍔 alimentacion%' no
	// matchea nada y la respuesta sale $0 sobre gastos que existen.
	if s := stripLeadingIcon(args.Search); s != "" {
		q.Search = &s
	}
	if args.Account != "" {
		accts, err := svc.QueryAccountsByUserID(userID)
		if err != nil {
			return movement.MovementQuery{}, err
		}
		var names []string
		for _, a := range accts {
			if strings.EqualFold(a.Name, args.Account) {
				id := uint64(a.ID)
				q.AccountID = &id
				break
			}
			names = append(names, a.Name)
		}
		// Un nombre que no matchea NO puede seguir de largo. Antes dejaba AccountID
		// en nil y la consulta corría sin filtrar: el usuario preguntaba por una
		// cuenta y le contestaban por todas, sin ninguna señal. Es el modo de falla
		// más caro de los tres del 2026-08-13, porque devuelve un número grande y
		// plausible en vez de un cero que llama la atención.
		//
		// El error vuelve al modelo como texto (AnswerQuery no aborta, lo reinyecta),
		// así que lleva las cuentas reales: con eso se corrige solo en la ronda
		// siguiente. Es lo mismo que ya hacía execAccountBalance más abajo.
		if q.AccountID == nil {
			return movement.MovementQuery{}, fmt.Errorf("no encontré la cuenta %q. Tus cuentas: %s",
				args.Account, strings.Join(names, ", "))
		}
	}
	return q, nil
}

// stripLeadingIcon saca el ícono que le antepusimos NOSOTROS al nombre de una
// categoría antes de usarlo como filtro.
//
// list_categories y sum_movements(group_by=category) devuelven "🍔 Alimentación",
// porque el prompt le pide al modelo que arranque la línea con el emoji. El modelo
// aprende el nombre de esa salida y lo copia entero al filtro de la llamada
// siguiente: el SQL compara contra "Alimentación" a secas, no matchea nada, y la
// respuesta sale "$0" sobre gastos que existen. Visto en el eval del 2026-08-13.
//
// Va acá y no en cada productor de íconos porque este es el embudo: los dos filtros
// de taxonomía de las dos tools pasan por buildMovementQuery.
//
// Corta hasta la primera letra o dígito, así un nombre real llega intacto —incluidos
// los que tienen espacios y barras, como "Deudas / préstamos"—.
func stripLeadingIcon(s string) string {
	for i, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return strings.TrimSpace(s[i:])
		}
	}
	// Sin una sola letra ni dígito no hay nombre que rescatar, y devolver ""
	// sería PEOR que no hacer nada: el llamador lee "" como "no vino filtro" y
	// la consulta pasa a correr sin filtrar, contestando por todo. Es el mismo
	// defecto que arregla la cuenta inexistente, un campo más allá.
	//
	// Un filtro nunca se ensancha en silencio: se deja como vino, la consulta
	// devuelve cero, y eso el modelo sí lo sabe explicar.
	return strings.TrimSpace(s)
}

func parseQueryDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", strings.TrimSpace(s))
}

// describeReminder renders a user's reminder for the QUERY loop to narrate.
// nil = no reminder configured. Windows shown as whole hours (ART).
func describeReminder(r *reminder.Reminder) string {
	if r == nil {
		return "El usuario no tiene ningún recordatorio de carga de gastos configurado."
	}
	estado := "activo"
	if !r.Enabled {
		estado = "apagado"
	}
	return fmt.Sprintf("Recordatorio de carga de gastos: %s. Franja: entre las %d y las %d (aviso alrededor de las %d:%02d, solo los días sin movimientos cargados).",
		estado, r.WindowStartMin/60, r.WindowEndMin/60, r.MidpointMin()/60, r.MidpointMin()%60)
}
