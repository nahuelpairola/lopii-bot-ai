package orchestrator

import "encoding/json"

const (
	ToolRecordMovements        = "record_movements"
	ToolFindMovementsToCorrect = "find_movements_to_correct"
	ToolCorrectMovement        = "correct_movement"
	ToolDeleteMovements        = "delete_movements"
	ToolManageAccount          = "manage_account"
	ToolAnswerQuery            = "answer_query"
	ToolManageSettings         = "manage_settings"
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

const schemaNoArgs = `{"type": "object", "properties": {}}`

var recordMovementsParams = json.RawMessage(`{
	"type": "object",
	"properties": {
		"movements": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"type": {"type": "string", "enum": ["expense", "income", "transfer"]},
					"amount": {"type": "string", "description": "positivo, EXCEPTO la pierna de un transfer que sale de una cuenta: esa va negativa. Las 2 piernas de un transfer suman 0"},
					"currency": {"type": "string", "enum": ["ARS", "USD"]},
					"account_id": {"type": ["integer", "null"]},
					"account_name_guess": {"type": ["string", "null"]},
					"payment_method": {"type": "string"},
					"description": {"type": "string"},
					"date": {"type": "string"},
					"group": {"type": ["string", "null"]}
				},
				"required": ["type", "amount", "currency", "payment_method", "description", "date"]
			}
		}
	},
	"required": ["movements"]
}`)

const SearchProperty = `"search": {"type": ["string", "null"], "description": "opcional: texto a buscar, sin distinguir mayúsculas ni acentos. Cada palabra tiene que aparecer en el nombre de la categoría, el de la subcategoría o la descripción del movimiento. Mantené las palabras que acotan: para el seguro de la moto pasá \"seguro moto\", no \"seguro\". Ej: \"alimentacion\", \"netflix\", \"lote\", \"seguro moto\"."}`

func AgentTools() []AgentTool {
	return []AgentTool{
		{
			Name:        ToolRecordMovements,
			When:        "cuenta un gasto, un ingreso o un movimiento de plata NUEVO. Un rendimiento de inversión también se registra acá. NO la uses si el mensaje se refiere a algo que ya cargó (\"al café de hoy\", \"eso que puse\", \"el del lote\"): eso es correct_movement.",
			Kind:        KindWrite,
			Description: "Registra uno o más movimientos financieros a partir del mensaje del usuario. Usala siempre que cuente un gasto, un ingreso o un movimiento de plata entre sus cuentas.",
			Parameters:  recordMovementsParams,
		},

		{
			Name:        ToolListCategories,
			When:        "preguntas por qué categorías existen.",
			Kind:        KindRead,
			Description: "Lista las categorías y subcategorías disponibles. Sin filtro devuelve el listado completo (categoría | subcategoría). Pasá category para acotarla a una sola categoría: ahí además viene la descripción de cuándo usar cada subcategoría. Usala cuando el usuario pregunta qué categorías existen o para qué sirve una.",
			Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"category": {"type": ["string", "null"], "description": "opcional: filtrar a una sola categoría"}
			}
		}`),
		},
		{
			Name:        ToolSumMovements,
			When:        "preguntas y resúmenes que se contestan con un total (\"cuánto gasté\").",
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
				"account": {"type": ["string", "null"], "description": "opcional: nombre de una cuenta del usuario"},
				` + SearchProperty + `
			},
			"required": ["from", "to", "currency"]
		}`),
		},
		{
			Name:        ToolListMovements,
			When:        "preguntas que se contestan listando movimientos concretos.",
			Kind:        KindRead,
			Description: "Lista movimientos individuales (los más recientes primero) en un rango de fechas, con filtros opcionales. Montos en positivo.",
			Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"from": {"type": "string", "description": "fecha desde YYYY-MM-DD"},
				"to": {"type": "string", "description": "fecha hasta YYYY-MM-DD"},
				"currency": {"type": "string", "enum": ["ARS", "USD"]},
				"type": {"type": ["string", "null"], "enum": ["expense", "income", "transfer", null]},
				"account": {"type": ["string", "null"]},
				` + SearchProperty + `,
				"limit": {"type": ["integer", "null"], "description": "máximo de filas (default 20, tope 50)"}
			},
			"required": ["from", "to", "currency"]
		}`),
		},
		{
			Name:        ToolAccountBalance,
			When:        "preguntas por el saldo de una cuenta.",
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
			When:        "SABER cómo tiene configurado el recordatorio (no para cambiarlo).",
			Kind:        KindRead,
			Description: "Devuelve el recordatorio diario de carga de gastos del usuario: si está activo o apagado y en qué franja horaria avisa. Usala cuando el usuario pregunta por su recordatorio (\"¿a qué hora me recordás?\", \"¿tengo recordatorio activo?\").",
			Parameters:  json.RawMessage(schemaNoArgs),
		},

		{
			Name:        ToolCorrectMovement,
			When:        "el mensaje toca un MOVIMIENTO ya registrado. Cuatro familias: reemplazo (\"en realidad eran 2000\", \"estaba mal\"), reintegro (\"me devolvieron 100\", \"me lo regalaron\", \"me reintegraron la mitad\"), INCREMENTO (\"sumale 1070\", \"agregale\", \"restale\", \"son X más\", \"al … de hoy\") y RE-UBICACIÓN, que no toca la plata: cambiarle la categoría, la cuenta o la fecha. El vocabulario que la marca es PONELO, MOVELO, VA, ERA, SALIÓ (\"ponelo en Vivienda\", \"movelo al banco X\", \"eso va en otra categoría\", \"era de la otra cuenta\", \"salió de la caja\", \"fue ayer\"). Un mensaje que nombra algo ya cargado y dice dónde va NO es un movimiento nuevo, aunque no traiga monto: no pidas el monto, ya lo tiene. No busques cuál: la app lo busca sola. Si el pedido es BORRARLO entero, usá delete_movements. FRONTERA: el monto de una CUENTA es un saldo, no un movimiento — \"modificá el monto de la cuenta X\", \"ajustá el saldo\", \"dejá la cuenta en 5000\" son manage_settings.",
			Kind:        KindAction,
			Description: "Corrige un movimiento ya registrado (monto, fecha, categoría, cuenta o descripción). La app busca sola de cuál habla el mensaje y le pide confirmación al usuario.",
			Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"change": {"type": "string", "description": "qué hay que cambiar, en palabras del usuario"},
				"scope": {"type": ["string", "null"], "enum": ["one", "all", null], "description": "'all' si el pedido abarca TODOS los movimientos que nombra (\"los movimientos del lote\", \"todos los de Carrefour\"); default 'one'"},
				"changes": {"type": "array", "description": "Qué cambia, en campos. Sólo lo que cambia, nunca el movimiento entero. \"Era pollo\" → [{field:description,value:pollo}] · \"eran 2000\" → [{field:amount,value:2000}] · \"sumale 1070\" → [{field:amount,op:add,value:1070}]. NUNCA hagas la cuenta vos: si te devolvieron LA MITAD mandá op=multiply value=0.5, no el resultado. Un reintegro RESTA: \"me devolvieron 500\" → op=subtract value=500; \"me devolvieron la mitad\" → op=multiply value=0.5; \"me lo regalaron\" → op=multiply value=0. Array VACÍO si el usuario pide editar sin decir qué (\"editá los movimientos de hoy\"): ahí la app le pregunta.", "items": {
					"type": "object",
					"properties": {
						"field": {"type": "string", "enum": ["category", "account", "date", "amount", "currency", "description", "type"]},
						"op": {"type": ["string", "null"], "enum": ["set", "add", "subtract", "multiply", null], "description": "SÓLO para amount (add/subtract/multiply). Omitilo en todo lo demás: se asume set"},
						"value": {"type": "string"}
					},
					"required": ["field", "value"]
				}},
				"date_from": {"type": ["string", "null"], "description": "Fecha que IDENTIFICA de cuál movimiento habla el mensaje, y SÓLO cuando el mensaje la dice con día y mes: \"el débito del 4 de agosto\", \"el del 12/07\". Si en cambio dice un día de la semana o algo relativo (\"el lunes\", \"ayer\", \"la semana pasada\"), dejala en null y no la calcules: la app resuelve eso sola y mejor. La usa para encontrar el movimiento; NO le cambia la fecha — si lo que se corrige ES la fecha, va en changes con field:date. YYYY-MM-DD o null."},
				"date_to": {"type": ["string", "null"], "description": "Fecha de fin cuando el mensaje da un TRAMO con las dos puntas dichas (\"los gastos del 3 al 5 de agosto\"): ahí van las dos. Un período relativo (\"la semana pasada\") NO se completa acá ni en date_from. YYYY-MM-DD o null; con date_from solo se busca en ese único día."}
			},
			"required": ["change", "changes"]
		}`),
		},
		{
			Name:        ToolDeleteMovements,
			When:        "pedido explícito de borrar (\"borrá\", \"eliminá\"). Tampoco busques cuál. Si en cambio hay que CAMBIARLE algo —incluso dejarlo en cero porque se lo regalaron— es correct_movement.",
			Kind:        KindAction,
			Description: "Borra uno o más movimientos ya registrados. La app busca sola de cuál habla el mensaje y le pide confirmación al usuario.",
			Parameters:  json.RawMessage(schemaNoArgs),
		},
		{
			Name:        ToolManageSettings,
			When:        "configurar sus CUENTAS, sus CATEGORÍAS o su RECORDATORIO. Incluye preguntar cómo los tiene configurados. Y todo lo que le pase al SALDO de una cuenta: crearla, renombrarla, ajustarle el monto, ponerla en cero, hacerla default (\"modificá el monto de la cuenta X\", \"corregí lo que tengo en X\"). Que aparezca un monto NO la vuelve una corrección de movimiento: lo que decide es si el monto es de una CUENTA o de un GASTO.",
			Kind:        KindAction,
			Description: "Abre la configuración de cuentas, categorías o recordatorio. Cubre crear, renombrar, ajustar saldo, fusionar, borrar, y también consultar cómo está configurado hoy.",
			Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"area": {"type": "string", "enum": ["cuenta", "categoria", "categoria_administrar", "recordatorio"], "description": "qué está configurando. 'categoria' = quiere UNA NUEVA; 'categoria_administrar' = sacar, borrar o fusionar una que ya tiene (\"eliminá subcategorías\", \"unificá estas dos\")"}
			},
			"required": ["area"]
		}`),
		},
		{
			Name:        ToolAnswerQuery,
			When:        "una PREGUNTA sobre plata ya registrada: cuánto gastó, en qué, saldos, comparaciones entre períodos.",
			Kind:        KindAction,
			Description: "Contesta una pregunta sobre los movimientos y saldos ya registrados. No lleva argumentos: la app le pasa la pregunta tal cual la escribió el usuario.",
			Parameters:  json.RawMessage(schemaNoArgs),
		},
		{
			Name:        ToolReplyHelp,
			When:        "\"¿qué podés hacer?\", \"¿cómo funcionás?\", o un saludo sin pedido concreto. Si el mensaje SÍ pide algo pero no se entiende, usá ask_rewrite.",
			Kind:        KindAction,
			Description: "Cuando el usuario pregunta qué podés hacer, cómo se usa el bot, o saluda sin pedir nada concreto.",
			Parameters:  json.RawMessage(schemaNoArgs),
		},
		{
			Name:        ToolAskRewrite,
			When:        "nada accionable (off-topic, gibberish, recetas) o falta lo esencial y ninguna otra herramienta aplica.",
			Kind:        KindAction,
			Description: "Cuando el mensaje no se entiende o le falta lo esencial y ninguna otra herramienta aplica. Pedirle que lo reescriba es mejor que adivinar.",
			Parameters:  json.RawMessage(schemaNoArgs),
		},
	}
}
