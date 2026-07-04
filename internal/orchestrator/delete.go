package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
)

const deleteSystemPrompt = `Sos un asistente que confirma si un mensaje pide borrar un movimiento financiero ya registrado.
Se te da un movimiento candidato (una o más filas de una misma transacción) y el mensaje del usuario.
Devolvé resolved=true si el mensaje claramente pide borrar este candidato, o resolved=false si no coincide o no es un pedido de borrado.
Si el mensaje menciona una fecha puntual o día relativo, completá mentioned_date_from con esa fecha en formato YYYY-MM-DD y dejá mentioned_date_to vacío, sea cual sea el valor de resolved.
Si el mensaje menciona un RANGO de fechas ("entre el 27 y el 29", "entre ayer y anteayer"), completá mentioned_date_from con el inicio del rango y mentioned_date_to con el fin, ambos en formato YYYY-MM-DD.`

var deleteTool = toolSchema{
	Name:        "resolve_delete",
	Description: "Determina si el mensaje pide borrar el movimiento candidato dado",
	Parameters: json.RawMessage(`{
		"type": "object",
		"properties": {
			"resolved": {"type": "boolean"},
			"mentioned_date_from": {"type": "string"},
			"mentioned_date_to": {"type": "string"}
		},
		"required": ["resolved"]
	}`),
}

func (o *Orchestrator) ResolveDelete(ctx context.Context, text string, candidate MovementCandidate) (DeleteResult, error) {
	userMessage := fmt.Sprintf("Movimiento candidato:\n%s\n\nMensaje del usuario: %q", buildCandidateBlock(candidate), text)

	raw, err := o.client.chatCompletion(ctx, o.deleteModel, deleteSystemPrompt, userMessage, deleteTool)
	if err != nil {
		return DeleteResult{}, fmt.Errorf("orchestrator: resolve delete: %w", err)
	}

	var result DeleteResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return DeleteResult{}, fmt.Errorf("orchestrator: parse delete result: %w", err)
	}
	return result, nil
}
