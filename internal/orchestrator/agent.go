package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// toolCallNames rinde los nombres de una vuelta para el log.
func toolCallNames(calls []loopToolCall) []string {
	names := make([]string, 0, len(calls))
	for _, c := range calls {
		names = append(names, c.Function.Name)
	}
	return names
}

// maxAgentIterations caps how many tool rounds Run executes before forcing a
// narration. Higher than AnswerQuery's 3 because the unified loop legitimately
// chains more: a compound message can read, write and park in one turn. Still a
// runaway guard, not the expected path — the common turn is one round.
const maxAgentIterations = 5

var ErrAgentMaxIterations = errors.New("orchestrator: agent loop exceeded max iterations")

// ErrAgentTurnDone lo devuelve el executor cuando la APP se queda con el turno:
// ya parkeó la acción, o ya tiene escrita la respuesta que va a salir. No es un
// error; es el executor diciendo "no me narres esto".
//
// Sin esto el turno cuesta dos llamadas: la primera elige la tool, y la segunda
// existe sólo para que el modelo narre algo que el gate va a decir igual. Y una
// vuelta no es barata — es el prompt ENTERO de nuevo. Medido en producción sobre
// una corrección: router 566 + agente 4.816 + agente 4.916 = 10.298 tokens
// contra un TPM de 8.000, así que la segunda llamada se comía un 429 y el
// mensaje del usuario terminaba en la cola en vez de en el gate.
var ErrAgentTurnDone = errors.New("orchestrator: agent turn done")

// El orden lectura-antes-de-escritura se BORRÓ con la etapa 5.
//
// Existía para que un total no se calculara sin las filas que estaban por
// insertarse. Este toolbox no tiene NINGUNA tool de lectura —record_movements
// es la única KindWrite y todo lo demás es una acción—, así que ordenaba un
// conjunto cuyos elementos comparten rango: no hacía nada.
//
// Y borrarlo saca una trampa en vez de volver a documentarla: kindRank metía un
// Kind SIN SETEAR en el mismo bucket que un KindRead explícito, así que una tool
// de escritura futura declarada sin Kind compilaba, corría, y se ejecutaba
// DESPUÉS de las lecturas — exactamente el bug contra el que advertía el
// comentario de la propia función. Ningún test lo cubría.
//
// Si una etapa futura vuelve a meter tools de lectura acá (por ejemplo plegando
// QUERY), esto vuelve CON el valor cero hecho irrepresentable, no como estaba.

// Run drives the unified agent loop: it sends the tools, executes every call a
// round emits via the caller's execute closure (scoped to the user), feeds each
// result back as a role:"tool" message, and repeats until the model narrates.
//
// It returns the narration to send to the user. What was written or parked is
// the caller's business — execute is the caller's closure, so it already knows.
//
// Three things differ from AnswerQuery, which stays untouched until stage 4:
//
//   - The turn cut: an assistant message carrying content ALONGSIDE tool_calls
//     means the model already said what it had to. The calls still run — their
//     side effects are wanted — and then the turn ends without spending another
//     round replaying the whole prefix.
//   - A cap of 5 rounds instead of 3.
//
// A tool executor error is fed back to the model as text, not aborted, so it
// can recover or explain.
func (o *Orchestrator) Run(ctx context.Context, systemPrompt, userText string, history []QueryTurn, tools []AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error) {
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

	for i := 0; i < maxAgentIterations; i++ {
		// Round 0 forces a call. Opening in "auto" risks the worst failure
		// mode: the model replying "listo, anoté tus $5.000" without ever
		// calling record_movements — silent data loss. reply_help and
		// ask_rewrite exist so every message has something to call.
		choice := "auto"
		if i == 0 {
			choice = "required"
		}
		assistant, err := o.client.chatCompletionLoop(ctx, callTypeAgent, o.agentModel, messages, toolDefs, choice, maxAgentCompletionTokens)
		if errors.Is(err, ErrNothingToExtract) && choice == "required" {
			// The model refused to call anything under tool_choice:"required",
			// and Groq turns that into a hard 400. Observed on real correction
			// messages ("Le erre eran 1500"): with 15 tools and a short,
			// referent-less message, gpt-oss-20b emits nothing at all.
			//
			// ask_rewrite exists precisely so every message has something to
			// call, but the model does not always reach for it. Rather than
			// fail the turn, ask once more in "auto": a turn may legitimately
			// end in a narrated question, which is the design's own escape.
			//
			// OJO ANTES DE BORRARLO POR INÚTIL: bajo TPM 8.000 este reintento no
			// entra NUNCA, y aun así hay que dejarlo. Groq cobra
			// Requested = prompt + max_completion_tokens, o sea ~7.200 por llamada:
			// dos en el mismo minuto son 14.400. Medido el 2026-08-10, los 4
			// reintentos observados (3 ese día, 1 el 09) devolvieron 429.
			//
			// Lo que lo hace load-bearing es justamente ese 429: es lo que dispara
			// handleGroqError → pendingjob → replay, y ese replay es el que termina
			// registrando el mensaje. Sin el reintento, el 400 sale como
			// ErrNothingToExtract, que startAgentLoop no matchea, y el turno muere
			// en msgSomethingBroke con el lote perdido. Cuesta 40ms (el 429 vuelve
			// antes de procesar, sin gastar tokens).
			//
			// Silent data loss is not reopened by this. The model already
			// declined to call a tool, so there was nothing to record; the risk
			// it now claims to have recorded something is what the prompt's
			// "no repitas el detalle" rule and an empty receipt guard against.
			assistant, err = o.client.chatCompletionLoop(ctx, callTypeAgent, o.agentModel, messages, toolDefs, "auto", maxAgentCompletionTokens)
		}
		if err != nil {
			return "", fmt.Errorf("orchestrator: agent run: %w", err)
		}

		// Sin esto el loop es una caja negra: llm_calls guarda cuántos tokens
		// costó cada vuelta, pero no QUÉ pidió el modelo, y sin eso un turno que
		// no hace nada es indistinguible de uno que hizo lo correcto.
		slog.InfoContext(ctx, "agent round",
			"round", i,
			"tools", toolCallNames(assistant.ToolCalls),
			"narrated", strings.TrimSpace(assistant.Content) != "",
		)

		if len(assistant.ToolCalls) == 0 {
			return assistant.Content, nil
		}

		messages = append(messages, assistant)
		turnDone := false
		for _, call := range assistant.ToolCalls {
			result, execErr := execute(call.Function.Name, json.RawMessage(call.Function.Arguments))
			switch {
			case errors.Is(execErr, ErrAgentTurnDone):
				// La app se queda con el turno. Se sigue ejecutando el resto de
				// la vuelta igual: el modelo eligió todas sus calls antes de ver
				// un solo resultado, y sus efectos se quieren.
				turnDone = true
			case execErr != nil:
				// Sin esto la vuelta que falla es INVISIBLE: el error se le devuelve
				// al modelo como texto y el turno sigue, así que en producción solo
				// se nota como una vuelta extra de ~4.200 tokens de prompt sin
				// ninguna línea que diga por qué. Es lo que pasó el 2026-08-08 con
				// las transferencias de dos piernas: el guard las rechazaba, el
				// modelo corregía en la vuelta 1, y entre las dos se pasaban del TPM.
				// Los sitios pre-loop ya loguean su rechazo (ver messaging.guardReason);
				// este camino se lo había salteado al migrar.
				slog.WarnContext(ctx, "agent tool failed",
					"round", i, "tool", call.Function.Name, "err", execErr)
				result = fmt.Sprintf("error: %v", execErr)
			}
			messages = append(messages, loopMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    result,
			})
		}

		// The cut. Checked AFTER executing, so the writes and parkings the
		// model asked for still happen — it narrated and acted in one message.
		// turnDone es la misma idea desde el otro lado: no narró, pero la app ya
		// sabe qué decir, así que la vuelta que viene no tiene nada que aportar.
		if turnDone || strings.TrimSpace(assistant.Content) != "" {
			return assistant.Content, nil
		}
	}

	// Cap reached while the model still wanted tools. Force one narration from
	// the results already gathered. Tools are omitted (nil, not toolDefs):
	// Groq 400s hard if the model attempts a call while tool_choice is "none".
	final, err := o.client.chatCompletionLoop(ctx, callTypeAgent, o.agentModel, messages, nil, "none", maxAgentCompletionTokens)
	if err != nil {
		return "", fmt.Errorf("orchestrator: agent run (final): %w", err)
	}
	if strings.TrimSpace(final.Content) == "" {
		return "", ErrAgentMaxIterations
	}
	return final.Content, nil
}
