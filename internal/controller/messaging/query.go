package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/reminder"
)

// queryTools are the read-only tools the QUERY loop composes. Invariants
// live in the executor (Go), not here — the model only picks tools + ranges.
// Optional params are declared nullable (`["string","null"]`) — the tool-
// calling models routinely emit an explicit `null` for an argument they don't
// want to set, and Groq validates arguments against the schema server-side, so
// a plain `"string"` type 400s on that null before the executor ever runs.
// json.Unmarshal of null leaves the Go zero value, so the executor already
// treats it as "absent". Only from/to/currency are required (never null).
var queryTools = []orchestrator.AgentTool{
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
	msgSearchOutOfRangeFmt = "sin movimientos con «%s» entre %s y %s. Sí hay con ese texto en otras fechas."

	// El término sólo matchea movimientos de categorías reservadas, que apply()
	// esconde de todo total de gastos e ingresos. Sin este mensaje la app diría
	// que "transferencia" no existe, sobre 12 movimientos reales.
	msgSearchOnlyInternalFmt = "«%s» sólo aparece en movimientos internos —transferencias entre tus cuentas, saldos iniciales, ajustes—, que no entran en los totales de gastos e ingresos."

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
func (c *controller) describeEmptyResult(q movement.MovementQuery, args queryToolArgs) (string, error) {
	if q.Search == nil {
		return msgQueryNoRowsInRange, nil
	}
	term := *q.Search

	wide := q
	wide.From, wide.To = searchProbeFrom, searchProbeTo
	if rows, err := c.movements.ListForUser(wide, 1); err == nil && len(rows) > 0 {
		return fmt.Sprintf(msgSearchOutOfRangeFmt, term, args.From, args.To), nil
	}

	// Sonda 2: las reservadas. apply() las excluye siempre salvo que se pidan, y
	// el ejecutor de query nunca las pide, así que la sonda 1 las esconde igual
	// que la consulta real. Sin esto, "transferencia" y "saldo inicial" —12 y 4
	// movimientos reales en la base local— se declararían inexistentes.
	wide.OnlyReserved = true
	if rows, err := c.movements.ListForUser(wide, 1); err == nil && len(rows) > 0 {
		return fmt.Sprintf(msgSearchOnlyInternalFmt, term), nil
	}

	return "", fmt.Errorf(msgSearchNotFoundFmt, term)
}

// handleQuery answers a read-only question via the agent loop. Returns
// (answered, err): answered=false significa que el loop no produjo respuesta.
//
// El fracaso NO manda copy acá — la manda el caller, a propósito. Un 429 se encola
// y se ackea (handleGroqError); si esta función mandara msgQueryFailed por su cuenta,
// el usuario leería "no pude responder" Y el ack de la cola por el mismo mensaje.
// Solo el caller sabe distinguir un 429 encolado de un fracaso de verdad.
func (c *controller) handleQuery(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) (bool, error) {
	prompt := c.buildQuerySystemPrompt()
	execute := c.buildQueryExecutor(userID)

	// Best-effort: a history load error never fails the query — run stateless.
	turns, _ := c.chatHistory.Recent(userID)
	history := make([]orchestrator.QueryTurn, len(turns))
	for i, t := range turns {
		history[i] = orchestrator.QueryTurn{Question: t.Question, Answer: t.Answer}
	}

	answer, err := c.orchestrator.AnswerQuery(ctx, prompt, text, history, queryTools, execute)
	if err != nil || strings.TrimSpace(answer) == "" {
		return false, err
	}
	c.sendText(ctx, b, chatID, answer)
	// Best-effort append: a failure here never fails the answer the user already got.
	_ = c.chatHistory.Append(userID, text, answer)
	return true, nil
}

func (c *controller) buildQuerySystemPrompt() string {
	today := startOfTodayArgentina().Format("2006-01-02")
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

// buildQueryExecutor returns the execute closure the loop calls per tool
// call. It is scoped to userID and owns every invariant (user-scoping, abs
// amounts, ARS/USD separation) — the LLM can only pick tools and ranges.
func (c *controller) buildQueryExecutor(userID uint64) func(string, json.RawMessage) (string, error) {
	return func(name string, raw json.RawMessage) (string, error) {
		var args queryToolArgs
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", fmt.Errorf("argumentos inválidos: %w", err)
		}
		switch name {
		case "list_categories":
			return c.execListCategories(userID, args)
		case "sum_movements":
			return c.execSumMovements(userID, args)
		case "list_movements":
			return c.execListMovements(userID, args)
		case "account_balance":
			return c.execAccountBalance(userID, args)
		case "get_reminder":
			rem, err := c.reminders.FindByUserID(userID)
			if err != nil {
				rem = nil // no row (or lookup miss) -> "no configurado"
			}
			return describeReminder(rem), nil
		default:
			return "", fmt.Errorf("herramienta desconocida: %s", name)
		}
	}
}

func (c *controller) execListCategories(userID uint64, args queryToolArgs) (string, error) {
	subs, err := c.subcategories.FindAllForUser(userID)
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

func (c *controller) execSumMovements(userID uint64, args queryToolArgs) (string, error) {
	q, err := c.buildMovementQuery(userID, args)
	if err != nil {
		return "", err
	}
	groupBy := args.GroupBy
	rows, err := c.movements.SumForUser(q, groupBy)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return c.describeEmptyResult(q, args)
	}
	cur := q.Currency.String()
	if groupBy == "" || groupBy == "none" {
		return fmt.Sprintf("total: %s %s", rows[0].Total.Abs().StringFixed(2), cur), nil
	}
	// For account grouping, map account_id labels to names.
	nameByID := map[string]string{}
	if groupBy == "account" {
		accts, _ := c.accounts.FindByUserID(userID)
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
			label = c.subcategories.IconForCategory(userID, label) + " " + label
		}
		lines = append(lines, fmt.Sprintf("%s: %s %s", label, r.Total.Abs().StringFixed(2), cur))
	}
	return strings.Join(lines, "\n"), nil
}

func (c *controller) execListMovements(userID uint64, args queryToolArgs) (string, error) {
	q, err := c.buildMovementQuery(userID, args)
	if err != nil {
		return "", err
	}
	ms, err := c.movements.ListForUser(q, args.Limit)
	if err != nil {
		return "", err
	}
	if len(ms) == 0 {
		return c.describeEmptyResult(q, args)
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

func (c *controller) execAccountBalance(userID uint64, args queryToolArgs) (string, error) {
	accts, err := c.accounts.FindByUserID(userID)
	if err != nil {
		return "", err
	}
	var lines []string
	for _, a := range accts {
		if args.Account != "" && !strings.EqualFold(a.Name, args.Account) {
			continue
		}
		bal, err := c.movements.SumAmountForAccount(uint64(a.ID))
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
func (c *controller) buildMovementQuery(userID uint64, args queryToolArgs) (movement.MovementQuery, error) {
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
		accts, err := c.accounts.FindByUserID(userID)
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
