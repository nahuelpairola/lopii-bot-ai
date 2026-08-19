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

// El orden lectura-antes-de-escritura se BORRÓ con la etapa 5 (a419524),
// argumentando que este toolbox no tenía tools de lectura.
//
// OJO: eso dejó de ser cierto el mismo día. 0b3427a agregó cinco KindRead, y la
// vuelta ejecuta TODAS las calls en el orden que eligió el modelo (ver el range
// de abajo), así que un sum_movements y un record_movements en la misma vuelta
// vuelven a ser la condición que el orden cubría. Sin medir si el modelo las
// mezcla, no se sabe si muerde: llm_calls.tool_calls tiene la respuesta.
//
// Si el orden vuelve, el valor cero de Kind tiene que ser irrepresentable: un
// Kind SIN SETEAR caía en el mismo bucket que un KindRead explícito, así que una
// tool de escritura declarada sin Kind se ejecutaba DESPUÉS de las lecturas.

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
		assistant, err := o.agentRound(ctx, messages, toolDefs, choice)
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
			assistant, err = o.agentRound(ctx, messages, toolDefs, "auto")
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
				// Los sitios pre-loop ya loguean su rechazo (ver flow.GuardReason);
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
	final, err := o.agentRound(ctx, messages, nil, "none")
	if err != nil {
		return "", fmt.Errorf("orchestrator: agent run (final): %w", err)
	}
	if strings.TrimSpace(final.Content) == "" {
		return "", ErrAgentMaxIterations
	}
	return final.Content, nil
}

// agentRound corre UNA ronda del loop, corriéndose de modelo cuando el
// principal rebota por cupo.
//
// Los techos de Groq son POR MODELO: leyendo los headers, gpt-oss-20b da 8.000
// TPM y gpt-oss-120b otros 8.000, cada uno con su TPD. Un 429 en uno no dice
// nada del otro, así que reintentar en el siguiente convierte una espera de ~40
// segundos en una respuesta inmediata, y multiplica la capacidad diaria por la
// cantidad de modelos de la cadena.
//
// SÓLO se corre ante un 429. Un 400 —un schema mal armado, un JSON cortado— es
// nuestro y sale igual en cualquier modelo: reintentarlo sería gastar el cupo de
// los suplentes para obtener el mismo error.
//
// El modelo suplente NO es equivalente y no se pretende que lo sea: es mejor una
// respuesta de otro modelo ahora que la del preferido dentro de 40 segundos. La
// vara está baja a propósito, porque la alternativa es esperar. Cada intento
// queda en `llm_calls` con SU modelo, así que con qué frecuencia se dispara la
// cadena —y si el suplente hace peor las cosas— se mide, no se supone.
func (o *Orchestrator) agentRound(ctx context.Context, messages []loopMessage, tools []toolDef, toolChoice string) (loopMessage, error) {
	chain := append([]string{o.agentModel}, o.agentFallbacks...)
	return o.roundWithFallback(ctx, callTypeAgent, chain, messages, tools, toolChoice, maxAgentCompletionTokens)
}

// roundWithFallback corre UNA llamada del loop recorriendo `chain` ante un 429.
// La comparten agentRound y AnswerQuery, que necesitaban lo mismo con distinto
// callType, distinta cadena y distinto cap de completion.
//
// Que sea UNA función y no dos gemelas es deliberado: `Run` y `AnswerQuery` ya son
// dos loops casi idénticos, y el riesgo documentado del paquete es arreglar un bug
// en uno y portarlo al otro por costumbre. La cadena es justo el tipo de lógica
// donde eso pasa — el loop de query estuvo sin ninguna hasta el 2026-08-13, y un
// turno rescatado en el agente moría igual un paso después, en la query.
//
// Una cadena de un solo modelo (fallbacks vacíos) se comporta exactamente como no
// tener cadena: una llamada, y el 429 sale para arriba a encolarse.
func (o *Orchestrator) roundWithFallback(ctx context.Context, callType string, chain []string,
	messages []loopMessage, tools []toolDef, toolChoice string, maxTokens int) (loopMessage, error) {
	var lastErr error
	for i, model := range chain {
		msg, err := o.client.chatCompletionLoop(ctx, callType, model, messages, tools, toolChoice, maxTokens)
		if err == nil {
			return msg, nil
		}
		var rateLimited *RateLimitedError
		if !errors.As(err, &rateLimited) {
			return msg, err
		}
		lastErr = err
		slog.WarnContext(ctx, "modelo sin cupo, probando el siguiente",
			"call_type", callType, "model", model, "restantes", len(chain)-i-1)
	}
	// Todos rebotaron: se devuelve el ÚLTIMO 429 para que el caller lo encole.
	// El RetryAfter del último es el más informativo — es el del modelo que se
	// probó más tarde.
	return loopMessage{}, lastErr
}
