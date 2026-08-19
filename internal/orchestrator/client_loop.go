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

// maxFirstRoundCompletionTokens capea SÓLO la primera ronda del loop de query, que
// corre con tool_choice:"required" y por lo tanto NO PUEDE narrar: está obligada a
// devolver una llamada a herramienta. Ahí no hay prosa que truncar, que es exactamente
// lo que impedía bajar maxQueryCompletionTokens.
//
// Medido sobre llm_calls (30 rondas de herramientas con HTTP 200): el máximo de
// completion fue 417 y ninguna pasó de 512. 640 deja 223 de margen sobre el peor caso
// observado, que además incluye los tokens de razonamiento de los gpt-oss.
const maxFirstRoundCompletionTokens = 640

// maxQueryCompletionTokens caps narration length in AnswerQuery. Se queda en 1.024:
// la que aprieta el TPM del loop de query es la cantidad de llamadas
// (maxQueryIterations), no este cap — bajarlo trunca respuestas reales, porque la
// narración normal sale de una ronda y no de la llamada final forzada.
//
// Aplica a las rondas 1+ y a la narración de la puerta 1. La ronda 0 va por
// maxFirstRoundCompletionTokens: es la única que no puede narrar.
const maxQueryCompletionTokens = 1024

// maxNarrationCompletionTokens capea la NARRACIÓN FORZADA, que es más barata que una
// ronda: el modelo ya tiene los datos y sólo redacta.
//
// Groq reserva prompt + max_completion_tokens contra el TPM aunque la respuesta no los
// use, así que un cap grande de más es cupo que se le saca a la consulta siguiente.
// Medido el 2026-08-13 contra Groq real, narrando la misma respuesta: 35-61 tokens en
// llama-3.3-70b —que era el modelo de narración entonces; Groq lo dio de baja el
// 2026-08-17 y hoy narra gpt-oss-20b, sin volver a medir— y 174-376 en los razonadores.
// 400 deja ~6 veces de margen sobre el caso medido.
const maxNarrationCompletionTokens = 400

// maxAgentCompletionTokens: techo de completion de Run. Groq cobra
// prompt+max_completion contra el cupo, se use o no, así que este número es
// cupo gastado en cada llamada.
//
// 1500 sale del peor lote real medido, con margen para 8-9 movimientos. Por qué
// NO 1.000 —y las mediciones que fijaron el número—:
// docs/decisions.md § Groq quota, the 429 queue and rate limits.
//
// Revisar si aparece un mensaje de más de 8 movimientos: ahí vuelve el 400 con
// el JSON cortado.
const maxAgentCompletionTokens = 1500

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
