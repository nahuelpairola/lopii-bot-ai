package orchestrator

import "encoding/json"

// Tool names. They are consts because agent_executor.go switches on them in
// stage 2, and a typo in a string literal there is a silent no-op, not a
// compile error.
const (
	ToolRecordMovements        = "record_movements"
	ToolFindMovementsToCorrect = "find_movements_to_correct"
	ToolCorrectMovement        = "correct_movement"
	ToolDeleteMovements        = "delete_movements"
	ToolManageAccount          = "manage_account"
	ToolCreateCategory         = "create_category"
	ToolManageCategories       = "manage_categories"
	ToolSetReminder            = "set_reminder"
	ToolReplyHelp              = "reply_help"
	ToolAskRewrite             = "ask_rewrite"
	ToolListCategories         = "list_categories"
	ToolSumMovements           = "sum_movements"
	ToolListMovements          = "list_movements"
	ToolAccountBalance         = "account_balance"
	ToolGetReminder            = "get_reminder"
)

// schemaNoArgs is the parameter schema for a tool that needs nothing from the
// model: the request itself is the whole signal, and the app collects any
// missing detail through ask_user.
const schemaNoArgs = `{"type": "object", "properties": {}}`

// AgentTools returns the 15 tools of spec §4.10, in a stable order.
//
// The five read tools and record_movements carry their existing schemas
// VERBATIM — from messaging/query.go's queryTools and create.go's createTool.
// Stage 1 changes no behaviour, and a "tidied" schema is a behaviour change:
// the read executors parse these exact argument names (queryToolArgs), and
// record_movements feeds the money path.
//
// The nullable unions ("string"/null) in the read schemas are deliberate.
// Tool-calling models routinely emit an explicit null for an argument they do
// not want to set, and Groq validates arguments against the schema server-side,
// so a plain "string" type 400s on that null before the executor ever runs.
func AgentTools() []AgentTool {
	return []AgentTool{
		// ---- write (exactly one) ----
		{
			Name:        ToolRecordMovements,
			Kind:        KindWrite,
			Description: "Registra uno o más movimientos financieros a partir del mensaje del usuario. Usala siempre que cuente un gasto, un ingreso o un movimiento de plata entre sus cuentas.",
			Parameters:  createTool.Parameters,
		},

		// ---- read ----
		{
			Name:        ToolFindMovementsToCorrect,
			Kind:        KindRead,
			Description: "Busca los movimientos que el usuario podría estar queriendo corregir o borrar, a partir de lo que dice el mensaje. Usala ANTES de correct_movement o delete_movements para saber de cuál está hablando.",
			Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"text": {"type": "string", "description": "el pedido del usuario tal como lo escribió, para buscar por descripción, comercio o monto"}
			},
			"required": ["text"]
		}`),
		},
		{
			Name:        ToolListCategories,
			Kind:        KindRead,
			Description: "Lista las categorías y subcategorías disponibles con la descripción de cuándo usar cada una. Usala cuando el usuario pregunta qué categorías existen o para qué sirve una.",
			Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"category": {"type": ["string", "null"], "description": "opcional: filtrar a una sola categoría"}
			}
		}`),
		},
		{
			Name:        ToolSumMovements,
			Kind:        KindRead,
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
			Name:        ToolListMovements,
			Kind:        KindRead,
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
			Name:        ToolAccountBalance,
			Kind:        KindRead,
			Description: "Saldo actual de una cuenta o de todas las cuentas del usuario. El saldo es la suma de sus movimientos. Nunca mezcla ARS y USD.",
			Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"account": {"type": ["string", "null"], "description": "opcional: nombre de una cuenta; sin esto, todas"}
			}
		}`),
		},
		{
			Name:        ToolGetReminder,
			Kind:        KindRead,
			Description: "Devuelve el recordatorio diario de carga de gastos del usuario: si está activo o apagado y en qué franja horaria avisa. Usala cuando el usuario pregunta por su recordatorio (\"¿a qué hora me recordás?\", \"¿tengo recordatorio activo?\").",
			Parameters:  json.RawMessage(schemaNoArgs),
		},

		// ---- action: each one parks the request; the app takes it from there ----
		{
			Name:        ToolCorrectMovement,
			Kind:        KindAction,
			Description: "Corrige un movimiento ya registrado (monto, fecha, categoría, cuenta o descripción). Llamá antes a find_movements_to_correct para saber cuál es.",
			Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"transaction_id": {"type": ["string", "null"], "description": "opcional: el id que devolvió find_movements_to_correct. Sin esto la app le pregunta al usuario cuál"},
				"change": {"type": "string", "description": "qué hay que cambiar, en palabras del usuario"}
			},
			"required": ["change"]
		}`),
		},
		{
			Name:        ToolDeleteMovements,
			Kind:        KindAction,
			Description: "Borra uno o más movimientos ya registrados. Llamá antes a find_movements_to_correct para saber cuáles.",
			Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"transaction_id": {"type": ["string", "null"], "description": "opcional: el id que devolvió find_movements_to_correct. Sin esto la app le pregunta al usuario cuál"}
			}
		}`),
		},
		{
			Name:        ToolManageAccount,
			Kind:        KindAction,
			Description: "Crear, renombrar, ajustar el saldo o dar de baja una cuenta del usuario.",
			Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"request": {"type": "string", "description": "qué quiere hacer con la cuenta, en palabras del usuario"}
			},
			"required": ["request"]
		}`),
		},
		{
			Name:        ToolCreateCategory,
			Kind:        KindAction,
			Description: "Cuando el usuario quiere crear una categoría o subcategoría nueva.",
			Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"request": {"type": "string", "description": "la categoría o subcategoría que quiere crear, en palabras del usuario"}
			},
			"required": ["request"]
		}`),
		},
		{
			Name:        ToolManageCategories,
			Kind:        KindAction,
			Description: "Cuando el usuario quiere fusionar, renombrar o borrar una subcategoría propia que ya existe.",
			Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"request": {"type": "string", "description": "qué quiere hacer con sus categorías, en palabras del usuario"}
			},
			"required": ["request"]
		}`),
		},
		{
			Name:        ToolSetReminder,
			Kind:        KindAction,
			Description: "Cuando el usuario quiere activar, cambiar o apagar su recordatorio diario de carga de gastos. Para SABER cómo lo tiene configurado usá get_reminder.",
			Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"request": {"type": "string", "description": "qué quiere hacer con el recordatorio, en palabras del usuario"}
			},
			"required": ["request"]
		}`),
		},
		{
			Name:        ToolReplyHelp,
			Kind:        KindAction,
			Description: "Cuando el usuario pregunta qué podés hacer, cómo se usa el bot, o saluda sin pedir nada concreto.",
			Parameters:  json.RawMessage(schemaNoArgs),
		},
		{
			Name:        ToolAskRewrite,
			Kind:        KindAction,
			Description: "Cuando el mensaje no se entiende o le falta lo esencial y ninguna otra herramienta aplica. Pedirle que lo reescriba es mejor que adivinar.",
			Parameters:  json.RawMessage(schemaNoArgs),
		},
	}
}
