package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
)

const routerSystemPrompt = `Sos un clasificador de intención para un bot de finanzas personales argentino. Devolvé la acción que mejor describe qué QUIERE el usuario. No juzgues si el mensaje trae datos completos: eso lo resuelve el paso siguiente.
- CREATE: reporta un movimiento nuevo. Verbo de gasto/cobro (gasté, pagué, cobré, compré, transferí), un débito en voz pasiva ("se debitó", "me debitaron", "salió de la cuenta"), o un monto/ítem suelto ("20k", "nafta"). Una cuenta que rinde o gana ("el plazo fijo rindió 5000") es CREATE. Monto Y verbo de movimiento ("transferí 50k a inversión") es CREATE aunque nombre una cuenta.
- UPDATE: corrige o contradice un movimiento YA registrado, o reporta un reintegro/devolución o un regalo/invitación sobre un ítem ya registrado. Señales: "en realidad"/"al final"/"era-eran-fue" corrigiendo un monto ("el café en realidad era 3000"); un reintegro que alude a algo previo ("me devolvió 100 por el café", "reintegro del super"); un ítem ya registrado que pasó a gratis ("al final me regalaron el helado"). Un reintegro o regalo NO es income nuevo: se resuelve corrigiendo el movimiento aludido. Ante duda CREATE/UPDATE cuando corrige algo propio, preferí UPDATE.
- DELETE: pide EXPLÍCITAMENTE borrar/eliminar un movimiento ya registrado ("borrá", "eliminá"). Reportar que salió/se debitó plata NO es DELETE, es CREATE.
- QUERY: pregunta o pide un resumen sobre movimientos existentes, sin registrar ni corregir. Preguntar por el recordatorio ("¿a qué hora me recordás?") es QUERY.
- ACCOUNT_MANAGE: crear una cuenta nueva o modificar una existente (renombrar, ajustar saldo, dejar en cero, elegir default). No registra un movimiento.
- CREATE_CATEGORY: crear una categoría o subcategoría nueva (una clasificación), no clasificar un movimiento.
- CATEGORY_MANAGE: sacar, borrar o unificar una categoría/subcategoría que YA existe ("borrá la categoría mascotas", "comida y alimentos son lo mismo").
- REMINDER_SET: crear, cambiar, activar o apagar el recordatorio diario de carga de gastos ("recordame cargar gastos", "apagá el aviso").
- HELP: pide ayuda o pregunta qué podés hacer.
- UNCLEAR: el mensaje no expresa ninguna intención accionable — no es un movimiento, consulta, corrección ni gestión (gibberish, off-topic, un saludo suelto). OJO: un monto o un ítem de gasto sin el otro dato ("20k", "nafta") SÍ es un movimiento → CREATE; los datos que falten los pide el paso siguiente.

Ejemplos:
- "se debitaron 610503 de la cuenta del banco" → CREATE (débito reportado)
- "me devolvió 100 por el café" → UPDATE (reintegro sobre algo existente)
- "me dieron 500 de aguinaldo" → CREATE (income nuevo, no alude a una compra previa)
- "al final me regalaron el helado" → UPDATE (el helado registrado pasó a gratis)
- "borrá el gasto del café" → DELETE
- "el plazo fijo rindió 5000" → CREATE (rendimiento)
- "¿a qué hora me recordás?" → QUERY
- "20k" → CREATE (monto suelto; el paso siguiente pide la categoría)
- "dame una receta de galletas" → UNCLEAR (sin intención accionable)`

var routerTool = toolSchema{
	Name:        "classify_intent",
	Description: "Clasifica la intención del mensaje del usuario",
	Parameters: json.RawMessage(`{
		"type": "object",
		"properties": {
			"intent": {"type": "string", "enum": ["CREATE", "UPDATE", "DELETE", "QUERY", "ACCOUNT_MANAGE", "CREATE_CATEGORY", "CATEGORY_MANAGE", "REMINDER_SET", "HELP", "UNCLEAR"]}
		},
		"required": ["intent"]
	}`),
}

type routerArgs struct {
	Intent Intent `json:"intent"`
}

func (o *Orchestrator) ClassifyIntent(ctx context.Context, text string) (IntentResult, error) {
	raw, err := o.client.chatCompletion(ctx, callTypeRouter, o.routerModel, routerSystemPrompt, text, routerTool)
	if err != nil {
		return IntentResult{}, fmt.Errorf("orchestrator: classify intent: %w", err)
	}

	var args routerArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return IntentResult{}, fmt.Errorf("orchestrator: parse intent: %w", err)
	}

	switch args.Intent {
	case IntentCreate, IntentUpdate, IntentDelete, IntentQuery, IntentAccountManage, IntentCreateCategory, IntentCategoryManage, IntentReminderSet, IntentHelp, IntentUnclear:
		return IntentResult{Intent: args.Intent}, nil
	default:
		return IntentResult{}, fmt.Errorf("orchestrator: unknown intent %q", args.Intent)
	}
}
