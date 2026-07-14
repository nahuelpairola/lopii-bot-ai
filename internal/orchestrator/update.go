package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
)

const updateSystemPromptTemplate = `Sos un asistente que corrige movimientos financieros ya registrados, a partir de un mensaje de corrección en lenguaje natural.
Se te da un movimiento candidato (una o más filas de una misma transacción) y el mensaje del usuario.
Si el mensaje claramente se refiere a este candidato, devolvé resolved=true y el set COMPLETO de movimientos corregido (todos los campos de todas las filas, no solo lo que cambia).
Si el mensaje no parece hablar de este candidato (menciona otro comercio, monto o fecha que no coincide), devolvé resolved=false y dejá movements vacío.
Si el mensaje menciona una fecha puntual o día relativo ("el lunes pasado", "el 3 de enero", "ayer"), completá mentioned_date_from con esa fecha en formato YYYY-MM-DD y dejá mentioned_date_to vacío, sea cual sea el valor de resolved.
Si el mensaje menciona un RANGO de fechas ("entre el 27 y el 29", "entre ayer y anteayer"), completá mentioned_date_from con el inicio del rango y mentioned_date_to con el fin, ambos en formato YYYY-MM-DD.
Si el mensaje es un REINTEGRO o DEVOLUCIÓN ("me devolvió 100 por el café", "me dieron 500 del asado"), el movimiento corregido conserva type/subcategory/currency/fecha del candidato y su amount es el amount original MENOS lo devuelto (café 700, reintegro 100 → 600). Nunca dejes el amount en el monto devuelto; siempre restá del original.
Si el ítem pasó a ser gratis en su totalidad (te lo regalaron o invitaron: "me regalaron el helado", "me invitaron el café", "el asado fue gratis"), el amount corregido es 0. La app interpreta un amount 0 como que ese movimiento se anula.
Si el mensaje corrige la cuenta de la que salió o entró la plata ("era Galicia", "fue de la cuenta X", "salió del banco", "era del fci"), buscá esa cuenta en la lista de CUENTAS DEL USUARIO de abajo y poné su account_id en TODAS las filas afectadas. Nunca inventes un account_id. Si el mensaje no menciona ninguna cuenta, conservá el account_id del candidato tal cual.

CUENTAS DEL USUARIO (id | nombre (moneda)):
%s`

func buildUpdateSystemPrompt(accounts []AccountOption) string {
	return fmt.Sprintf(updateSystemPromptTemplate, buildAccountsBlock(accounts))
}

var updateTool = toolSchema{
	Name:        "resolve_and_correct",
	Description: "Determina si el mensaje corrige el movimiento candidato dado y devuelve el set corregido",
	Parameters: json.RawMessage(`{
		"type": "object",
		"properties": {
			"resolved": {"type": ["boolean", "string"]},
			"mentioned_date_from": {"type": ["string", "null"]},
			"mentioned_date_to": {"type": ["string", "null"]},
			"movements": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"type": {"type": "string", "enum": ["expense", "income", "transfer"]},
						"amount": {"type": "string"},
						"currency": {"type": "string", "enum": ["ARS", "USD"]},
						"account_id": {"type": ["integer", "null"]},
						"category": {"type": "string"},
						"subcategory": {"type": "string"},
						"payment_method": {"type": "string"},
						"merchant": {"type": ["string", "null"]},
						"description": {"type": "string"},
						"date": {"type": "string"},
						"group": {"type": ["string", "null"]}
					},
					"required": ["type", "amount", "currency", "category", "subcategory", "payment_method", "description", "date"]
				}
			}
		},
		"required": ["resolved", "movements"]
	}`),
}

func buildCandidateBlock(candidate MovementCandidate) string {
	b, _ := json.Marshal(candidate)
	return string(b)
}

type updateArgs struct {
	Resolved          flexBool        `json:"resolved"`
	MentionedDateFrom string          `json:"mentioned_date_from,omitempty"`
	MentionedDateTo   string          `json:"mentioned_date_to,omitempty"`
	Movements         []MovementDraft `json:"movements"`
}

func (o *Orchestrator) ResolveUpdate(ctx context.Context, text string, candidate MovementCandidate, accounts []AccountOption) (UpdateResult, error) {
	userMessage := fmt.Sprintf("Movimiento candidato:\n%s\n\nMensaje del usuario: %q", buildCandidateBlock(candidate), text)

	raw, err := o.client.chatCompletion(ctx, callTypeUpdate, o.updateModel, buildUpdateSystemPrompt(accounts), userMessage, updateTool)
	if err != nil {
		return UpdateResult{}, fmt.Errorf("orchestrator: resolve update: %w", err)
	}

	var args updateArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return UpdateResult{}, fmt.Errorf("orchestrator: parse update result: %w", err)
	}
	return UpdateResult{
		Resolved:          bool(args.Resolved),
		MentionedDateFrom: args.MentionedDateFrom,
		MentionedDateTo:   args.MentionedDateTo,
		Movements:         args.Movements,
	}, nil
}
