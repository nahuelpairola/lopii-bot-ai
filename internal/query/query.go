package query

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/agent"
	"lopiibot.com/internal/chathistory"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
)

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
	QuerySendText(ctx context.Context, chat messenger.Chat, text string)
	AnswerQuery(ctx context.Context, systemPrompt, userText string, history []orchestrator.QueryTurn, tools []orchestrator.AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error)
}

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

type queryToolArgs struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Currency string `json:"currency"`
	GroupBy  string `json:"group_by"`
	Type     string `json:"type"`
	Account  string `json:"account"`
	Search   string `json:"search"`
	Limit    int    `json:"limit"`
	Category string `json:"category"`
}

func Run(ctx context.Context, svc services, chat messenger.Chat, userID uint64, text string) (bool, error) {
	prompt := SystemPrompt()

	var appVerdict, rangeFrom, rangeTo string
	inner := NewExecutor(svc, userID)
	execute := func(name string, raw json.RawMessage) (string, error) {
		out, err := inner(name, raw)
		if strings.Contains(out, msgOnlyInternalMark) {
			appVerdict = out
		}
		if emptyResultNamesARange(out) {
			var args queryToolArgs
			if json.Unmarshal(raw, &args) == nil {
				rangeFrom, rangeTo = friendlyDate(args.From), friendlyDate(args.To)
			}
		}
		return out, err
	}

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
	svc.QuerySendText(ctx, chat, answer)
	_ = svc.QueryChatAppend(userID, text, answer)
	return true, nil
}

func SystemPrompt() string {
	now := agent.StartOfTodayArgentina()
	today := movement.WeekdayEs(now) + " " + now.Format("2006-01-02")
	return fmt.Sprintf(`Sos el asistente de consultas de un bot de finanzas personales argentino.
Basá TODA cifra en los datos que devuelven las herramientas — nunca inventes ni estimes un número sin respaldo de una herramienta.
Sí podés hacer aritmética SOBRE esos datos: sumar, restar, promediar o sacar tasas por día/mes. Para un promedio mensual, pedí los totales por mes (group_by=month) y dividí. Para comparar dos períodos ("cuánto más que el mes pasado"), pedí cada total y restá. Para una tasa diaria, dividí el total por la cantidad de días del rango.
Hoy es %s (hora de Argentina). Resolvé fechas relativas ("hoy", "ayer", "esta semana", "el mes pasado", "mayo") a rangos concretos YYYY-MM-DD antes de llamar una herramienta.
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
				rem = nil
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
	withDescriptions := args.Category != ""
	var lines []string
	for _, s := range subs {
		if args.Category != "" && s.Category != args.Category {
			continue
		}
		line := fmt.Sprintf("%s | %s", s.Category, s.Subcategory)
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
	transferSplit := args.Type == constants.Transfer && ungroupedSum(groupBy)
	if transferSplit {
		groupBy = movement.GroupByDirection
	}
	rows, err := svc.QuerySumMovements(q, groupBy)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 || (ungroupedSum(groupBy) && rows[0].Total.IsZero()) || (transferSplit && allZero(rows)) {
		return describeEmptyResult(svc, q, args)
	}
	cur := q.Currency.String()
	if transferSplit {
		name, err := accountName(svc, userID, args.Account)
		if err != nil {
			return "", err
		}
		return renderTransferDirections(rows, name, cur), nil
	}
	if ungroupedSum(groupBy) {
		return fmt.Sprintf("total: %s %s", rows[0].Total.Abs().StringFixed(2), cur), nil
	}
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
	if line, ok := groupedTotalLine(rows, groupBy, cur, args.Type); ok {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

const (
	msgTransferOutFmt     = "salió: %s %s"
	msgTransferInFmt      = "entró: %s %s"
	msgTransferOutAcctFmt = "salió de %s: %s %s"
	msgTransferInAcctFmt  = "entró a %s: %s %s"
)

const (
	dirOut = "out"
	dirIn  = "in"
)

func renderTransferDirections(rows []movement.CategorySum, account, cur string) string {
	totals := map[string]decimal.Decimal{dirOut: decimal.Zero, dirIn: decimal.Zero}
	for _, r := range rows {
		totals[r.Label] = r.Total.Abs()
	}
	out, in := totals[dirOut].StringFixed(2), totals[dirIn].StringFixed(2)
	if account == "" {
		return fmt.Sprintf(msgTransferOutFmt, out, cur) + "\n" + fmt.Sprintf(msgTransferInFmt, in, cur)
	}
	return fmt.Sprintf(msgTransferOutAcctFmt, account, out, cur) + "\n" +
		fmt.Sprintf(msgTransferInAcctFmt, account, in, cur)
}

func allZero(rows []movement.CategorySum) bool {
	for _, r := range rows {
		if !r.Total.IsZero() {
			return false
		}
	}
	return true
}

func ungroupedSum(groupBy string) bool {
	return groupBy == movement.GroupByNone || groupBy == groupByNoneArg
}

const groupByNoneArg = "none"

func groupedTotalLine(rows []movement.CategorySum, groupBy, cur, movType string) (string, bool) {
	if groupBy == "type" || movType == constants.Transfer || len(rows) < 2 {
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

func buildMovementQuery(svc services, userID uint64, args queryToolArgs) (movement.MovementQuery, error) {
	cur := currency.Currency(args.Currency)
	if cur != currency.ARS && cur != currency.USD {
		cur = currency.ARS
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
	if s := stripLeadingIcon(args.Search); s != "" {
		q.Search = &s
	}
	if args.Account != "" {
		accts, err := svc.QueryAccountsByUserID(userID)
		if err != nil {
			return movement.MovementQuery{}, err
		}
		acct, names := matchAccount(accts, args.Account)
		if acct != nil {
			id := uint64(acct.ID)
			q.AccountID = &id
		}
		if q.AccountID == nil {
			return movement.MovementQuery{}, fmt.Errorf("no encontré la cuenta %q. Tus cuentas: %s",
				args.Account, strings.Join(names, ", "))
		}
	}
	return q, nil
}

func matchAccount(accts []account.Account, name string) (*account.Account, []string) {
	names := make([]string, 0, len(accts))
	for i := range accts {
		if strings.EqualFold(accts[i].Name, name) {
			return &accts[i], nil
		}
		names = append(names, accts[i].Name)
	}
	return nil, names
}

func accountName(svc services, userID uint64, asked string) (string, error) {
	if asked == "" {
		return "", nil
	}
	accts, err := svc.QueryAccountsByUserID(userID)
	if err != nil {
		return "", err
	}
	if acct, _ := matchAccount(accts, asked); acct != nil {
		return acct.Name, nil
	}
	return "", nil
}

func stripLeadingIcon(s string) string {
	for i, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return strings.TrimSpace(s[i:])
		}
	}
	return strings.TrimSpace(s)
}

func parseQueryDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", strings.TrimSpace(s))
}

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
