package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
)

const routerSystemPrompt = `Sos un clasificador de intención para un bot de finanzas personales argentino.
Clasificá el mensaje del usuario en una de estas 5 acciones:
- CREATE: el mensaje reporta un movimiento nuevo. Típicamente tiene un verbo de acción (gasté, pagué, cobré, transferí, compré) o es un monto+categoría suelto sin verbo (ej. "20k", "nafta 5k").
- UPDATE: el mensaje corrige el monto, categoría u otro dato de un movimiento YA registrado. Señal clave: verbo copulativo en pasado (era/eran/fue/fueron) describiendo un monto, CON o SIN marcador explícito de contraste — con marcador (ej. "el café en realidad era 3000", "eran 150 usd no 100") y también SIN ningún marcador (ej. "el café de hoy eran 5k", "el gasto de nafta fue 8000").
- DELETE: el mensaje pide borrar o eliminar un movimiento ya registrado.
- QUERY: el mensaje pregunta o pide un resumen/consulta sobre movimientos existentes, sin registrar ni corregir nada.
- ACCOUNT_CREATE: el mensaje pide crear una cuenta nueva (no un movimiento) — billetera, cuenta de inversión, jubilación, ahorro, etc. Señal clave: menciona "cuenta"/"cuentas" sin montos ni verbos de movimiento (gasté, pagué, cobré, transferí). Ejemplos: "quiero crear una cuenta nueva", "nueva cuenta", "cuentas", "abrí una cuenta para mi jubilación", "quiero agregar una cuenta de inversión". Un mensaje con monto Y cuenta (ej. "transferí 50k a mi cuenta de inversión") sigue siendo CREATE, no ACCOUNT_CREATE — ahí ya existe un flujo que ofrece crear la cuenta si no existe.
- CREATE_CATEGORY: el mensaje pide crear una categoría o subcategoría nueva, no registrar/corregir/borrar un movimiento ni consultar. Señal clave: menciona "categoría"/"subcategoría" en el sentido de crear una clasificación nueva, no de elegir una existente para un movimiento. Ejemplos: "quiero crear una categoría nueva", "quiero agregar una subcategoría", "necesito una categoría para mis gastos de mascotas".
Ante duda entre CREATE y UPDATE por un verbo copulativo en pasado (era/eran/fue) sin verbo de acción, preferí UPDATE.
Elegí siempre la que mejor describe la intención real del usuario.
Además, si clasificaste CREATE, marcá needs_confirmation=true únicamente cuando
el mensaje sea un monto aislado sin ningún otro dato que lo acompañe — ni verbo,
ni comercio, ni ítem, ni categoría (ej. "20k", "15000"). Si el mensaje tiene
CUALQUIER palabra además del monto (verbo de acción, nombre de comercio, ítem
comprado, categoría), needs_confirmation debe ser false, incluso sin verbo
explícito.

Ejemplos:
- "20k" → needs_confirmation=true (monto aislado, sin verbo ni ítem)
- "15000" → needs_confirmation=true (monto aislado)
- "nafta 5k" → needs_confirmation=false (tiene ítem/categoría)
- "panadería 10k" → needs_confirmation=false (tiene comercio)
- "compré pan 10k" → needs_confirmation=false (tiene verbo e ítem)
- "transferencia 50k" → needs_confirmation=false (tiene categoría)

Este criterio aplica solo si clasificaste CREATE. En cualquier otro caso
(incluido cualquier intent que no sea CREATE), needs_confirmation debe ser
false.`

var routerTool = toolSchema{
	Name:        "classify_intent",
	Description: "Clasifica la intención del mensaje del usuario",
	Parameters: json.RawMessage(`{
		"type": "object",
		"properties": {
			"intent": {"type": "string", "enum": ["CREATE", "UPDATE", "DELETE", "QUERY", "ACCOUNT_CREATE", "CREATE_CATEGORY"]},
			"needs_confirmation": {"type": ["boolean", "string"]}
		},
		"required": ["intent"]
	}`),
}

type routerArgs struct {
	Intent            Intent   `json:"intent"`
	NeedsConfirmation flexBool `json:"needs_confirmation"`
}

func (o *Orchestrator) ClassifyIntent(ctx context.Context, text string) (IntentResult, error) {
	raw, err := o.client.chatCompletion(ctx, o.routerModel, routerSystemPrompt, text, routerTool)
	if err != nil {
		return IntentResult{}, fmt.Errorf("orchestrator: classify intent: %w", err)
	}

	var args routerArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return IntentResult{}, fmt.Errorf("orchestrator: parse intent: %w", err)
	}

	switch args.Intent {
	case IntentCreate, IntentUpdate, IntentDelete, IntentQuery, IntentAccountCreate, IntentCreateCategory:
		return IntentResult{Intent: args.Intent, NeedsConfirmation: bool(args.NeedsConfirmation)}, nil
	default:
		return IntentResult{}, fmt.Errorf("orchestrator: unknown intent %q", args.Intent)
	}
}
