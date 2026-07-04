package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
)

const routerSystemPrompt = `Sos un clasificador de intención para un bot de finanzas personales argentino.
Clasificá el mensaje del usuario en una de estas 4 acciones:
- CREATE: el mensaje reporta un movimiento nuevo. Típicamente tiene un verbo de acción (gasté, pagué, cobré, transferí, compré) o es un monto+categoría suelto sin verbo (ej. "20k", "nafta 5k").
- UPDATE: el mensaje corrige el monto, categoría u otro dato de un movimiento YA registrado. Señal clave: verbo copulativo en pasado (era/eran/fue/fueron) describiendo un monto, CON o SIN marcador explícito de contraste — con marcador (ej. "el café en realidad era 3000", "eran 150 usd no 100") y también SIN ningún marcador (ej. "el café de hoy eran 5k", "el gasto de nafta fue 8000").
- DELETE: el mensaje pide borrar o eliminar un movimiento ya registrado.
- QUERY: el mensaje pregunta o pide un resumen/consulta sobre movimientos existentes, sin registrar ni corregir nada.
Ante duda entre CREATE y UPDATE por un verbo copulativo en pasado (era/eran/fue) sin verbo de acción, preferí UPDATE.
Elegí siempre la que mejor describe la intención real del usuario.
Además, si clasificaste CREATE, marcá needs_confirmation=true cuando el mensaje sea demasiado corto o ambiguo para confiar en él (ej. un monto pelado sin categoría ni verbo, tipo "20k"). En cualquier otro caso (incluido cualquier intent que no sea CREATE), needs_confirmation debe ser false.`

var routerTool = toolSchema{
	Name:        "classify_intent",
	Description: "Clasifica la intención del mensaje del usuario",
	Parameters: json.RawMessage(`{
		"type": "object",
		"properties": {
			"intent": {"type": "string", "enum": ["CREATE", "UPDATE", "DELETE", "QUERY"]},
			"needs_confirmation": {"type": "boolean"}
		},
		"required": ["intent"]
	}`),
}

type routerArgs struct {
	Intent            Intent `json:"intent"`
	NeedsConfirmation bool   `json:"needs_confirmation"`
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
	case IntentCreate, IntentUpdate, IntentDelete, IntentQuery:
		return IntentResult{Intent: args.Intent, NeedsConfirmation: args.NeedsConfirmation}, nil
	default:
		return IntentResult{}, fmt.Errorf("orchestrator: unknown intent %q", args.Intent)
	}
}
