package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
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
		Description: "Lista las categorías y subcategorías disponibles con la descripción de cuándo usar cada una. Usala cuando el usuario pregunta qué categorías existen o para qué sirve una.",
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
				"category": {"type": ["string", "null"]},
				"subcategory": {"type": ["string", "null"]},
				"account": {"type": ["string", "null"], "description": "opcional: nombre de una cuenta del usuario"},
				"merchant": {"type": ["string", "null"], "description": "opcional: nombre de comercio (coincidencia parcial, ej. Carrefour)"}
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
				"category": {"type": ["string", "null"]},
				"subcategory": {"type": ["string", "null"]},
				"account": {"type": ["string", "null"]},
				"merchant": {"type": ["string", "null"], "description": "opcional: nombre de comercio (coincidencia parcial)"},
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
}

// queryToolArgs is the union of every tool's argument shape — one struct
// keeps the executor's json.Unmarshal simple (unused fields stay zero).
type queryToolArgs struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Currency    string `json:"currency"`
	GroupBy     string `json:"group_by"`
	Type        string `json:"type"`
	Category    string `json:"category"`
	Subcategory string `json:"subcategory"`
	Account     string `json:"account"`
	Merchant    string `json:"merchant"`
	Limit       int    `json:"limit"`
}

// handleQuery answers a read-only question via the agent loop. Returns
// (answered, err): err (or an empty answer) means the loop failed and the
// caller should tell the user and resolve the metric as failed.
func (c *controller) handleQuery(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) (bool, error) {
	prompt := c.buildQuerySystemPrompt()
	execute := c.buildQueryExecutor(userID)

	// Best-effort: a history load error never fails the query — run stateless.
	turns, _ := c.queryHistory.Recent(userID)
	history := make([]orchestrator.QueryTurn, len(turns))
	for i, t := range turns {
		history[i] = orchestrator.QueryTurn{Question: t.Question, Answer: t.Answer}
	}

	answer, err := c.orchestrator.AnswerQuery(ctx, prompt, text, history, queryTools, execute)
	if err != nil || strings.TrimSpace(answer) == "" {
		c.sendText(ctx, b, chatID, msgQueryFailed)
		return false, err
	}
	c.sendText(ctx, b, chatID, answer)
	// Best-effort append: a failure here never fails the answer the user already got.
	_ = c.queryHistory.Append(userID, text, answer)
	return true, nil
}

func (c *controller) buildQuerySystemPrompt() string {
	today := startOfTodayArgentina().Format("2006-01-02")
	return fmt.Sprintf(`Sos el asistente de consultas de un bot de finanzas personales argentino.
Basá TODA cifra en los datos que devuelven las herramientas — nunca inventes ni estimes un número sin respaldo de una herramienta.
Sí podés hacer aritmética SOBRE esos datos: sumar, restar, promediar o sacar tasas por día/mes. Para un promedio mensual, pedí los totales por mes (group_by=month) y dividí. Para comparar dos períodos ("cuánto más que el mes pasado"), pedí cada total y restá. Para una tasa diaria, dividí el total por la cantidad de días del rango.
Hoy es %s (zona America/Argentina/Buenos_Aires). Resolvé fechas relativas ("hoy", "ayer", "esta semana", "el mes pasado", "mayo") a rangos concretos YYYY-MM-DD antes de llamar una herramienta.
Los montos se muestran siempre en positivo. ARS y USD son mundos separados: nunca los sumes ni los conviertas; si hacen falta ambos, reportá cada uno por su lado.
Nunca hagas una pregunta de aclaración — no podés recibir la respuesta del usuario. Si la consulta es ambigua entre varias categorías o cuentas conocidas, resolvela vos: usá list_categories para ver las que aplican y respondé TODAS las interpretaciones plausibles en la misma respuesta, marcando "sin registros" las que no tengan datos.
Cuando tengas los datos, respondé en español rioplatense, claro y breve.
No uses Markdown ni caracteres decorativos: nada de *, **, _, #, ni guiones largos como separadores — Telegram los muestra crudos. Escribí texto plano, prolijo y bien organizado: líneas cortas, un ítem por línea cuando enumeres.
Montos en formato argentino: separador de miles con punto y símbolo adelante ($5.500, $1.234,56); no muestres los centavos ".00"/",00" cuando el monto es entero de pesos. Aclará la moneda (ARS/USD) cuando haga falta.
Fechas en formato amable (01/07 o "1 de julio"), nunca 2026-07-01.
Cuando una herramienta te da un emoji junto a una categoría, poné ese emoji al principio de la línea para que se lea visual.
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
	var lines []string
	for _, s := range subs {
		if args.Category != "" && s.Category != args.Category {
			continue
		}
		line := fmt.Sprintf("%s | %s | %s", s.Category, s.Subcategory, s.Description)
		if s.Icon != "" {
			line = s.Icon + " " + line
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return "No hay categorías que coincidan.", nil
	}
	return "categoría | subcategoría | cuándo usarla:\n" + strings.Join(lines, "\n"), nil
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
		return "Sin movimientos en ese rango.", nil
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
		return "Sin movimientos en ese rango.", nil
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
	if args.Category != "" {
		q.Category = &args.Category
	}
	if args.Subcategory != "" {
		q.Subcategory = &args.Subcategory
	}
	if args.Merchant != "" {
		q.Merchant = &args.Merchant
	}
	if args.Account != "" {
		accts, err := c.accounts.FindByUserID(userID)
		if err != nil {
			return movement.MovementQuery{}, err
		}
		for _, a := range accts {
			if strings.EqualFold(a.Name, args.Account) {
				id := uint64(a.ID)
				q.AccountID = &id
				break
			}
		}
	}
	return q, nil
}

func parseQueryDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", strings.TrimSpace(s))
}
