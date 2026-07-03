package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
)

const routerSystemPrompt = `Sos un clasificador de intención para un bot de finanzas personales argentino.
Clasificá el mensaje del usuario en una de estas 4 acciones:
- CREATE: el mensaje describe un movimiento financiero nuevo (gasto, ingreso, transferencia, inversión).
- UPDATE: el mensaje corrige o modifica un movimiento ya registrado (ej. "el café en realidad era 3000", "eran 150 usd no 100").
- DELETE: el mensaje pide borrar o eliminar un movimiento ya registrado.
- QUERY: el mensaje pregunta o pide un resumen/consulta sobre movimientos existentes, sin registrar ni corregir nada.
Elegí siempre la que mejor describe la intención real del usuario.`

var routerTool = toolSchema{
	Name:        "classify_intent",
	Description: "Clasifica la intención del mensaje del usuario",
	Parameters: json.RawMessage(`{
		"type": "object",
		"properties": {
			"intent": {"type": "string", "enum": ["CREATE", "UPDATE", "DELETE", "QUERY"]}
		},
		"required": ["intent"]
	}`),
}

type routerArgs struct {
	Intent Intent `json:"intent"`
}

func (o *Orchestrator) ClassifyIntent(ctx context.Context, text string) (Intent, error) {
	raw, err := o.client.chatCompletion(ctx, o.routerModel, routerSystemPrompt, text, routerTool)
	if err != nil {
		return "", fmt.Errorf("orchestrator: classify intent: %w", err)
	}

	var args routerArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", fmt.Errorf("orchestrator: parse intent: %w", err)
	}

	switch args.Intent {
	case IntentCreate, IntentUpdate, IntentDelete, IntentQuery:
		return args.Intent, nil
	default:
		return "", fmt.Errorf("orchestrator: unknown intent %q", args.Intent)
	}
}
