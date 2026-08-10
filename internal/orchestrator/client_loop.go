package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
)

// loopToolCall is one tool call inside an assistant message during the
// multi-tool agent loop.
type loopToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function loopToolCallFunc `json:"function"`
}

type loopToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// loopMessage is a chat message for the agent loop. Unlike chatMessage
// (single-shot, system+user only), it also carries an assistant message's
// tool_calls and a tool-result message's tool_call_id, so the full history
// can be replayed to the model each turn. Empty fields are omitted so a
// plain system/user/assistant/tool message serializes cleanly.
type loopMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []loopToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type loopRequest struct {
	Model               string        `json:"model"`
	Messages            []loopMessage `json:"messages"`
	Tools               []toolDef     `json:"tools"`
	ToolChoice          string        `json:"tool_choice"`
	Temperature         float64       `json:"temperature"`
	MaxCompletionTokens int           `json:"max_completion_tokens,omitempty"`
}

// EL MODELO DE COSTO DE GROQ, que gobierna las dos constantes de abajo y el cap de
// iteraciones de cada loop (maxQueryIterations en query.go, maxAgentIterations en
// agent.go). Leerlo antes de tocar cualquiera de esos números.
//
// Groq NO cobra contra el TPM lo que el modelo escribe: cobra
//
//	Requested = prompt_tokens + max_completion_tokens
//
// reservado por adelantado, se use o no. Un cap de 1.024 que en la práctica narra
// 176 tokens igual descuenta 1.024 del cupo del minuto. Por eso el costo real de un
// loop es (cantidad de llamadas) × (prompt + cap), y no lo que se lee en
// llm_calls.total_tokens, que mide el uso y no la reserva.
//
// Las dos veces que esto explotó en producción fue por no tener la cuenta a mano:
// el 2026-08-08 con el agent loop (cap 4096 → 8.286 > 8.000, TODAS las llamadas
// 429eaban) y el 2026-08-10 con el loop de query (4 llamadas × ~2.300 = ~9.200, con
// el bucket lleno y un solo usuario). El techo era 8.000 TPM por modelo en las dos.
//
// La cuenta se hace contra prompt_tokens REAL de la tabla llm_calls, por call_type,
// no contra una estimación.

// maxQueryCompletionTokens caps narration length in AnswerQuery. Se queda en 1.024:
// la que aprieta el TPM del loop de query es la cantidad de llamadas
// (maxQueryIterations), no este cap — bajarlo trunca respuestas reales, porque la
// narración normal sale de una ronda y no de la llamada final forzada.
const maxQueryCompletionTokens = 1024

// maxAgentCompletionTokens is Run's own cap. Higher than the query loop's
// because of the turn cut: one assistant message may carry every tool call of a
// round plus the final narration.
//
// Medido el 2026-08-10 en producción (prompt del agente ≈ 4.190 con 5 tools
// cableadas, TPM 8.000):
//
//	cap 3000 → 7.190. Entra, y deja lugar para el router del mensaje siguiente (~660).
//	cap 4096 → 8.286 > 8.000: TODAS las llamadas 429ean, no algunas.
//
// Se subió de 2048 a 3000 porque a 2048 un lote de 7 movimientos quedaba en el
// filo: el que entró usó 1.949 tokens de completion, y los dos intentos previos
// del MISMO mensaje volvieron 400 con el JSON cortado a la mitad.
//
// Cuando muera el router (etapa 5) esos ~660 se liberan pero el prompt unificado
// engorda con las 9 tools que hoy no se cablean (~1.100 tokens de schema):
// RECALCULAR el cap ahí contra el prompt_tokens real, no heredar este.
const maxAgentCompletionTokens = 3000

type loopResponse struct {
	Choices []struct {
		Message loopMessage `json:"message"`
	} `json:"choices"`
}

// chatCompletionLoop sends one Groq request with the given tool_choice and the
// full message history, and returns the assistant message — which carries
// either tool_calls (the loop must execute and feed back) or final content.
// toolChoice is "auto" for normal rounds and "none" on a forced-narration
// final call (Groq's documented way to make the model emit text instead of
// another tool round).
func (c *Client) chatCompletionLoop(ctx context.Context, callType, model string, messages []loopMessage, tools []toolDef, toolChoice string, maxTokens int) (loopMessage, error) {
	reqBody := loopRequest{
		Model:               model,
		Messages:            messages,
		Tools:               tools,
		ToolChoice:          toolChoice,
		Temperature:         0.1,
		MaxCompletionTokens: maxTokens,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return loopMessage{}, fmt.Errorf("orchestrator: marshal loop request: %w", err)
	}

	body, err := c.send(ctx, callType, model, payload)
	if err != nil {
		return loopMessage{}, err
	}

	var parsed loopResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return loopMessage{}, fmt.Errorf("orchestrator: unmarshal loop response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return loopMessage{}, fmt.Errorf("orchestrator: groq loop response had no choices")
	}
	return parsed.Choices[0].Message, nil
}
