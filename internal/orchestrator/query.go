package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// AgentToolKind is a tool's execution class. It decides the order the loop
// runs a round's calls in (see agent.go orderCallsByKind) — never the order
// the model happened to list them in.
type AgentToolKind string

const (
	// KindRead queries the DB and returns a string. Idempotent.
	KindRead AgentToolKind = "read"
	// KindWrite mutates. Only record_movements, and only when it resolves clean.
	KindWrite AgentToolKind = "write"
	// KindAction parks an intent on the queue and touches nothing itself.
	KindAction AgentToolKind = "action"
)

// AgentTool is a tool exposed to the agent loop. Same shape as toolSchema,
// but public so the messaging controller can define the tools and hold the
// executor — the orchestrator stays dependency-free (no repo imports).
type AgentTool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
	// Kind classes the tool for execution order. The zero value sorts as
	// KindRead, which is what the read-only AnswerQuery tools want.
	Kind AgentToolKind
	// When es la línea de "CUÁNDO USAR ESTA HERRAMIENTA" que BuildAgentPrompt
	// mete en el system prompt. Vive acá, y no suelta en la plantilla, porque
	// el prompt tiene que hablar EXACTAMENTE de las tools que se mandan: si
	// nombra una que no está, el modelo la pide igual y el turno se cae.
	// Vacía, la tool no aparece en el bloque (las de AnswerQuery no lo usan).
	When string
}

// QueryTurn is one prior question/answer pair fed back to the loop so a
// follow-up keeps its referent. Textual context only — tools re-run every
// call, so numbers are always fresh. The messaging controller maps
// chathistory.Turn to this (keeps the orchestrator dependency-free).
type QueryTurn struct {
	Question string
	Answer   string
}

// maxQueryIterations caps how many tool rounds the loop runs before giving up.
// Una consulta sana es 1 ronda de tools + 1 narración.
//
// El número NO es libre: es lo que decide cuántas llamadas a Groq puede hacer una
// consulta, y cada llamada reserva contra el TPM (ver el modelo de costo en
// client_loop.go). Medido el 2026-08-10 en producción, con prompts de 1.173 a 1.370
// y maxQueryCompletionTokens en 1.024 → ~2.300 reservados por llamada, TPM 8.000:
//
//	cap 2 → 3 llamadas = ~6.850. Entra, y deja lugar para el router del mensaje siguiente (~670).
//	cap 3 → 4 llamadas = ~9.200 > 8.000: la última llamada 429ea SIEMPRE, con el bucket lleno.
//
// Se bajó de 3 a 2 tras el incidente del 2026-08-10, donde cuatro consultas seguidas
// murieron con 429 sin que hubiera otro tráfico en cuatro horas. No le saca ninguna
// ronda a ninguna consulta que haya funcionado: en 14 días, ninguna consulta exitosa
// pasó de 3 llamadas. La 4ª solo existió para fallar.
const maxQueryIterations = 2

var ErrQueryMaxIterations = errors.New("orchestrator: query loop exceeded max iterations")

// AnswerQuery runs the read-only agent loop: it sends the tools with
// tool_choice:"auto", executes every tool call the model emits in a round
// (via the caller's execute closure, scoped to the user), feeds each result
// back as a tool message, and repeats. A tool executor error is fed back to the
// model as an error string, not aborted, so the model can recover or explain.
//
// LA RESPUESTA FINAL SALE POR DOS PUERTAS, y las dos son normales:
//
//  1. Salida por ronda: una ronda devuelve contenido en vez de tool calls. Es el
//     camino COMÚN — 16 de 22 consultas medidas narran así (ronda 0 pide tools,
//     ronda 1 narra).
//  2. Salida forzada: se agotó el cap con el modelo todavía pidiendo tools, así que
//     una última llamada con tool_choice:"none" lo obliga a narrar con lo que juntó.
//     Solo una respuesta final vacía devuelve ErrQueryMaxIterations.
//
// Quien toque los caps de tokens tiene que tener las dos en la cabeza: capear las
// rondas "porque solo eligen tools" trunca la respuesta real de la mayoría de las
// consultas. Ese razonamiento ya se intentó y estaba mal.
func (o *Orchestrator) AnswerQuery(ctx context.Context, systemPrompt, userText string, history []QueryTurn, tools []AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error) {
	toolDefs := make([]toolDef, len(tools))
	for i, t := range tools {
		toolDefs[i] = toolDef{
			Type:     "function",
			Function: toolFunction{Name: t.Name, Description: t.Description, Parameters: t.Parameters},
		}
	}

	messages := []loopMessage{{Role: "system", Content: systemPrompt}}
	for _, t := range history {
		messages = append(messages,
			loopMessage{Role: "user", Content: t.Question},
			loopMessage{Role: "assistant", Content: t.Answer},
		)
	}
	messages = append(messages, loopMessage{Role: "user", Content: userText})

	// toolResults junta en texto plano lo que devolvió cada tool, para poder narrar
	// desde ellos en la puerta 2 SIN reenviar el historial de tool_calls. Ver el
	// bloque de la narración forzada más abajo.
	var toolResults []string

	for i := 0; i < maxQueryIterations; i++ {
		// Force a tool call on the first round: weak models (8b-instant)
		// sometimes deflect ("no puedo darte una respuesta exacta") without
		// ever calling a tool. "required" guarantees the loop gathers real
		// data before it is allowed to narrate; later rounds go back to "auto".
		choice := "auto"
		if i == 0 {
			choice = "required"
		}
		assistant, err := o.client.chatCompletionLoop(ctx, callTypeQuery, o.queryModel, messages, toolDefs, choice, maxQueryCompletionTokens)
		if err != nil {
			return "", fmt.Errorf("orchestrator: answer query: %w", err)
		}
		if len(assistant.ToolCalls) == 0 {
			return assistant.Content, nil
		}
		messages = append(messages, assistant)
		for _, call := range assistant.ToolCalls {
			result, execErr := execute(call.Function.Name, json.RawMessage(call.Function.Arguments))
			if execErr != nil {
				result = fmt.Sprintf("error: %v", execErr)
			}
			messages = append(messages, loopMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    result,
			})
			toolResults = append(toolResults, result)
		}
	}

	// Puerta 2 (ver el doc comment): se agotó el cap con el modelo todavía pidiendo
	// tools. Se fuerza una narración (tool_choice:"none") para que resuma con lo que
	// ya juntó. No es la puerta de siempre: la mayoría de las consultas salen por la
	// puerta 1, narrando desde una ronda.
	//
	// El request final se arma LIMPIO —system + pregunta + datos en texto plano—, sin
	// reenviar los tool_calls ni los mensajes de rol "tool" del loop. Sacar solo los
	// schemas (tools: nil) NO alcanza: el modelo imita el historial de tool calls y
	// emite una tool call igual, y Groq la rechaza con 400 "Tool choice is none, but
	// model called a tool" (visto en producción el 2026-08-10 con gpt-oss-120b). Sin
	// rastro de tools en el historial, no hay nada que imitar. Lo fija
	// TestAnswerQuery_FinalNarration_HistoryHasNoToolTrace.
	finalMessages := []loopMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userText},
		{Role: "user", Content: "Datos obtenidos de las herramientas:\n" + strings.Join(toolResults, "\n") + "\n\nRedactá la respuesta final para el usuario con estos datos."},
	}
	final, err := o.client.chatCompletionLoop(ctx, callTypeQuery, o.queryModel, finalMessages, nil, "none", maxQueryCompletionTokens)
	if err != nil {
		return "", fmt.Errorf("orchestrator: answer query (final): %w", err)
	}
	if strings.TrimSpace(final.Content) == "" {
		return "", ErrQueryMaxIterations
	}
	return final.Content, nil
}
