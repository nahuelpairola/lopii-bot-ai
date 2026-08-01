package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
)

// routerSystemPrompt es var y no const porque embebe routerTieBreakers(): los
// desempates viven en tie_breakers.go, compartidos con el prompt unificado del
// agent loop. El texto renderizado es idéntico al que estaba escrito acá a
// mano, y TestRouterTieBreakers_TextIsUnchanged lo fija.
var routerSystemPrompt = `Sos un clasificador de intenciones para un bot de finanzas personales argentino.

Clasificá el mensaje del usuario en UNA sola intención. No des explicaciones.

INTENTS:
- CREATE: Nuevo movimiento (gasto, ingreso, transferencia, débito, rendimiento de plazo fijo, etc.). Incluye montos sueltos ("20k", "nafta").
- UPDATE: Corrección, reintegro, devolución o regalo sobre un movimiento previo ("en realidad", "me devolvieron", "al final me regalaron").
- DELETE: Pedido explícito de borrar un movimiento ("borrá", "eliminá").
- QUERY: Preguntas, resúmenes o consultas sobre movimientos/cuentas.
- ACCOUNT_MANAGE: Crear, renombrar, ajustar saldo o configurar cuentas.
- CREATE_CATEGORY: Crear una categoría nueva.
- CATEGORY_MANAGE: Borrar, modificar o unificar categorías existentes.
- REMINDER_SET: Gestionar recordatorios diarios.
- HELP: Pedir ayuda o explicación.
- UNCLEAR: Nada accionable (saludos, off-topic, gibberish, recetas, etc.).

Reglas rápidas:
` + routerTieBreakers() + `

Responde solo con la tool.`

var routerTool = toolSchema{
	Name:        "classify_intent",
	Description: "Analiza el mensaje del usuario y devuelve la intención exacta según las reglas definidas en el system prompt.",
	Parameters: json.RawMessage(`{
		"type": "object",
		"properties": {
			"intent": {
				"type": "string",
				"enum": ["CREATE", "UPDATE", "DELETE", "QUERY", "ACCOUNT_MANAGE", "CREATE_CATEGORY", "CATEGORY_MANAGE", "REMINDER_SET", "HELP", "UNCLEAR"],
				"description": "Debe ser exactamente una de estas opciones. No inventes ninguna otra."
			}
		},
		"required": ["intent"],
		"additionalProperties": false
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
