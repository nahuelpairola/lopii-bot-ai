package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// AgentToolKind is a tool's execution class. It decides the order the loop
// runs a round's calls in — never the order
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
// Una consulta sana es 1 ronda de herramientas + 1 narración, y sale por la puerta 1.
//
// Estuvo en 2 desde el 2026-08-10 y subió a 3 el 2026-08-14. La aritmética que
// justificaba el 2 —"cap 3 son 4 llamadas contra un TPM de 8.000, así que la última
// 429ea siempre"— venció en tres puntos, los tres medidos:
//
//   - La narración forzada SE FUE del bucket: usa narrationChain() (llama-3.3-70b) con
//     techo 400 y un prompt limpio, 788 tokens medidos. Con cap 3 el bucket de
//     gpt-oss-120b ve 3 llamadas, no 4.
//   - El router que reservaba ~670 para el mensaje siguiente lo borró la etapa 5.
//   - Un 429 dejó de ser fatal: roundWithFallback camina la cadena. El 2026-08-14, 20
//     de 52 llamadas rebotaron y no se perdió ninguna consulta.
//
// Presupuesto con cap 3 contra el bucket de 8.000, caso promedio (prompt de primera
// ronda 1.209 y +320 por ronda, medidos sobre llm_calls):
//
//	(1.209+640) + (1.529+1.024) + (1.849+1.024) = 7.275 → entra con ~9% de aire.
//
// En p95 (prompt 2.409) se pasa, y se acepta: la tercera ronda es rara —el prompt pide
// agrupar las herramientas en una sola ronda— y pasarse cuesta un salto de modelo, no
// la respuesta.
//
// El 21% de los turnos agotaba el cap de 2 (11 de 53 medidos), y en esa puerta vive el
// bug que este cambio acompaña: la narración forzada afirmaba ausencia sobre lo que
// nunca se consultó.
const maxQueryIterations = 3

var ErrQueryMaxIterations = errors.New("orchestrator: query loop exceeded max iterations")

// queryChain es el modelo de consultas seguido de sus suplentes. Las DOS llamadas de
// AnswerQuery la usan —las rondas y la narración forzada—: hasta el 2026-08-13 las dos
// iban directo contra queryModel y el primer 429 mataba el turno.
//
// Ese día quedó en las trazas el caso que lo justifica: un turno cuyo agente FUE
// rescatado (20b 429 → 120b 429 → llama-3.3-70b 200) murió un paso después, en la
// query, por no tener lo mismo que lo acababa de salvar.
func (o *Orchestrator) queryChain() []string {
	return append([]string{o.queryModel}, o.queryFallbacks...)
}

// narrationChain es el modelo de redacción seguido de los suplentes de query, sin
// repetir ninguno: reintentar el mismo modelo que acaba de rebotar por cupo no
// compra nada.
//
// narrationModel vacío devuelve queryChain() tal cual, así que un entorno que no
// declare el campo cae en el MODELO de query — pero el techo de completion de la
// narración sigue siendo 400, no los 1024 de antes: eso no depende de la cadena.
func (o *Orchestrator) narrationChain() []string {
	if o.narrationModel == "" {
		return o.queryChain()
	}
	chain := []string{o.narrationModel}
	for _, m := range o.queryChain() {
		if m != o.narrationModel {
			chain = append(chain, m)
		}
	}
	return chain
}

// describeCall rinde una llamada a tool como texto PLANO, para que el dato que
// produjo viaje identificado hasta la narración forzada.
//
// El formato NO es negociable y la restricción es una sola: no puede parecerse a
// una llamada a función. La primera versión concatenaba el JSON crudo de los
// argumentos —`sum_movements {"currency":"ARS",...}`— y costó un 400 en producción
// el 2026-08-13: el modelo lo IMITÓ y emitió una tool call, que con
// tool_choice:"none" Groq rechaza. El failed_generation lo mostró textual:
//
//	{"name": "repo_browser.run_code", "arguments": {"tool": "sum_movements", ...}}
//
// Es el mismo mecanismo que documenta TestAnswerQuery_FinalNarration_HistoryHasNoToolTrace
// —el modelo imita lo que ve— pero por otra puerta: ese test cuida el HISTORIAL de
// mensajes, y esto entra por el TEXTO. Sin llaves, sin comillas y sin paréntesis, no
// hay nada que imitar.
//
// Las claves van ordenadas para que dos resultados de la misma consulta se lean
// comparables, y los nulos se omiten: el schema obliga al modelo a mandar los
// opcionales en null, y "category=null" es ruido que encima invita a razonar sobre
// un filtro que nadie puso.
func describeCall(name, args string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(args), &m); err != nil || len(m) == 0 {
		return name
	}
	keys := make([]string, 0, len(m))
	for k, v := range m {
		if v == nil || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return name
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, m[k]))
	}
	return name + " con " + strings.Join(parts, ", ")
}

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
		// La ronda 0 va forzada a llamar una herramienta Y con un techo de completion
		// más chico: las dos cosas por la misma razón, que ahí no puede haber prosa.
		//
		// "required" porque los modelos flojos a veces esquivan ("no puedo darte una
		// respuesta exacta") sin llamar ninguna tool — lo midió llama-3.1-8b-instant,
		// que Groq da de baja el 2026-08-16 y este repo ya no usa, pero la regla aplica
		// a cualquier suplente barato que entre por la cadena. Garantiza que el loop
		// junte datos reales antes de poder narrar.
		//
		// El techo chico se sigue de lo mismo: una ronda que no puede narrar no tiene
		// respuesta que truncar. Las rondas siguientes vuelven a "auto" y a su techo.
		choice, capTokens := "auto", maxQueryCompletionTokens
		if i == 0 {
			choice, capTokens = "required", maxFirstRoundCompletionTokens
		}
		assistant, err := o.roundWithFallback(ctx, callTypeQuery, o.queryChain(), messages, toolDefs, choice, capTokens)
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
			// El resultado viaja CON su llamada, no suelto. En el camino normal la
			// asociación la da el ToolCallID del mensaje de arriba; acá no hay nada
			// que la sostenga, y toolResults es lo único que ve la narración forzada.
			//
			// El 2026-08-13, en producción, sin esto: el usuario preguntó por tres
			// cosas, el modelo alcanzó a consultar dos, y la puerta 2 recibió
			// "total: 9990.00 ARS" y "total: 8122.73 ARS" — dos números anónimos y
			// una pregunta que nombraba tres. Contestó con el total de HBO puesto
			// bajo la etiqueta del lote. No era alucinación: con esa entrada,
			// acertar la atribución es imposible.
			//
			// El formato es texto plano y NO puede parecerse a una llamada a
			// función. Ver describeCall: mandar los argumentos como JSON crudo
			// costó un 400 en producción el 2026-08-13.
			toolResults = append(toolResults, describeCall(call.Function.Name, call.Function.Arguments)+" → "+result)
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
		// Que los datos están INCOMPLETOS no es una precaución: es un hecho que Go sabe
		// con certeza. A este punto se llega SÓLO si el cap de rondas se agotó con el
		// modelo todavía pidiendo herramientas, o sea que quedó algo sin averiguar. No
		// hace falta parsear la pregunta ni adivinar qué faltó.
		//
		// Sin decirlo, el modelo rellena el hueco NEGANDO: el 2026-08-14 contestó "No
		// hay registros de Cuota préstamo en agosto" sobre $80.000 reales, habiendo
		// consultado dos de las tres cosas que le preguntaron.
		{Role: "user", Content: "Datos que se juntaron:\n" + strings.Join(toolResults, "\n") +
			"\n\nOJO: están INCOMPLETOS, quedaron cosas sin averiguar. Redactá la respuesta final " +
			"para el usuario usando SÓLO lo que está acá arriba. De lo que te hayan preguntado y no " +
			"aparezca en esta lista, decí que no llegaste a averiguarlo: no afirmes que no existe, " +
			"que no hay, ni que dio cero."},
	}
	final, err := o.roundWithFallback(ctx, callTypeQuery, o.narrationChain(), finalMessages, nil, "none", maxNarrationCompletionTokens)
	if err != nil {
		return "", fmt.Errorf("orchestrator: answer query (final): %w", err)
	}
	if strings.TrimSpace(final.Content) == "" {
		return "", ErrQueryMaxIterations
	}
	return final.Content, nil
}
