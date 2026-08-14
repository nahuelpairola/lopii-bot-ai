package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newQueryOrchestrator(url string) *Orchestrator {
	return New(Config{APIKey: "k", BaseURL: url, QueryModel: "test-model", TimeoutSeconds: 5})
}

func TestAnswerQuery_ExecutesToolThenReturnsContent(t *testing.T) {
	call := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		w.Header().Set("Content-Type", "application/json")
		if call == 1 {
			w.Write([]byte(`{"choices":[{"message":{"content":null,"tool_calls":[
				{"id":"c1","type":"function","function":{"name":"sum_movements","arguments":"{\"currency\":\"ARS\"}"}}
			]}}]}`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"Gastaste 5000 ARS.","tool_calls":null}}]}`))
	}))
	defer server.Close()

	var gotName string
	var gotArgs string
	execute := func(name string, args json.RawMessage) (string, error) {
		gotName = name
		gotArgs = string(args)
		return "total: 5000", nil
	}

	o := newQueryOrchestrator(server.URL)
	answer, err := o.AnswerQuery(context.Background(), "system", "cuánto gasté", nil,
		[]AgentTool{{Name: "sum_movements", Parameters: json.RawMessage(`{"type":"object"}`)}}, execute)
	if err != nil {
		t.Fatalf("AnswerQuery: %v", err)
	}
	if gotName != "sum_movements" {
		t.Errorf("executed tool = %q, want sum_movements", gotName)
	}
	if gotArgs != `{"currency":"ARS"}` {
		t.Errorf("executed args = %q", gotArgs)
	}
	if answer != "Gastaste 5000 ARS." {
		t.Errorf("answer = %q", answer)
	}
	if call != 2 {
		t.Errorf("groq calls = %d, want 2 (tool round + narration)", call)
	}
}

func TestAnswerQuery_MaxIterationsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never stops calling tools.
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":null,"tool_calls":[
			{"id":"c","type":"function","function":{"name":"sum_movements","arguments":"{}"}}
		]}}]}`))
	}))
	defer server.Close()

	execute := func(name string, args json.RawMessage) (string, error) { return "x", nil }
	o := newQueryOrchestrator(server.URL)
	_, err := o.AnswerQuery(context.Background(), "s", "u", nil,
		[]AgentTool{{Name: "sum_movements", Parameters: json.RawMessage(`{}`)}}, execute)
	if !errors.Is(err, ErrQueryMaxIterations) {
		t.Fatalf("err = %v, want ErrQueryMaxIterations", err)
	}
}

func TestAnswerQuery_ForcedFinalNarrationOnCap(t *testing.T) {
	call := 0
	var finalToolChoice string
	var finalToolsCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		var req loopRequest
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		if call <= maxQueryIterations {
			w.Write([]byte(`{"choices":[{"message":{"content":null,"tool_calls":[
				{"id":"c","type":"function","function":{"name":"sum_movements","arguments":"{}"}}
			]}}]}`))
			return
		}
		finalToolChoice = req.ToolChoice
		finalToolsCount = len(req.Tools)
		w.Write([]byte(`{"choices":[{"message":{"content":"Acá va el resumen.","tool_calls":null}}]}`))
	}))
	defer server.Close()

	execute := func(name string, args json.RawMessage) (string, error) { return "x", nil }
	o := newQueryOrchestrator(server.URL)
	answer, err := o.AnswerQuery(context.Background(), "s", "u", nil,
		[]AgentTool{{Name: "sum_movements", Parameters: json.RawMessage(`{}`)}}, execute)
	if err != nil {
		t.Fatalf("AnswerQuery: %v", err)
	}
	if answer != "Acá va el resumen." {
		t.Errorf("answer = %q", answer)
	}
	if finalToolChoice != "none" {
		t.Errorf("final tool_choice = %q, want none (forced narration)", finalToolChoice)
	}
	if finalToolsCount != 0 {
		t.Errorf("final round tools = %d, want 0 (no tool schemas => nothing for the model to call)", finalToolsCount)
	}
}

// Regresión del 2026-08-10 (segunda vuelta, en producción): la narración forzada
// volvía 400 "Tool choice is none, but model called a tool" contra gpt-oss-120b.
//
// La causa NO era mandar schemas en el request — eso ya se sacaba (tools: nil). Era
// el HISTORIAL: la llamada final reenviaba toda la conversación con sus tool_calls y
// sus mensajes de rol "tool", y el modelo IMITA ese patrón y emite una tool call
// igual, aunque no tenga schema. Groq valida la salida contra tool_choice:"none" y
// rechaza con 400. Sacar los schemas ataca el request; la causa está en lo que el
// historial le sugiere al modelo.
//
// Este stub reproduce la condición REAL de Groq: 400 si el request final trae CUALQUIER
// rastro de tools —un mensaje con tool_calls o de rol "tool"—, no si trae schemas. El
// fix arma un request final limpio (system + pregunta + datos en texto plano), así el
// modelo no tiene nada que imitar.
func TestAnswerQuery_FinalNarration_HistoryHasNoToolTrace(t *testing.T) {
	call := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		var req loopRequest
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		if call <= maxQueryIterations {
			w.Write([]byte(`{"choices":[{"message":{"content":null,"tool_calls":[
				{"id":"c","type":"function","function":{"name":"sum_movements","arguments":"{}"}}
			]}}]}`))
			return
		}
		// Llamada de narración forzada: si el historial trae rastro de tools, Groq
		// 400ea igual que en producción (el modelo imita y llama una tool).
		for _, m := range req.Messages {
			if m.Role == "tool" || len(m.ToolCalls) > 0 {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":{"message":"Tool choice is none, but model called a tool","code":"tool_use_failed"}}`))
				return
			}
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"Resumen sin herramientas.","tool_calls":null}}]}`))
	}))
	defer server.Close()

	execute := func(name string, args json.RawMessage) (string, error) { return "total: 5000 ARS", nil }
	o := newQueryOrchestrator(server.URL)
	answer, err := o.AnswerQuery(context.Background(), "s", "u", nil,
		[]AgentTool{{Name: "sum_movements", Parameters: json.RawMessage(`{}`)}}, execute)
	if err != nil {
		t.Fatalf("AnswerQuery: %v (la narración forzada no puede reenviar historial con tools)", err)
	}
	if answer != "Resumen sin herramientas." {
		t.Errorf("answer = %q", answer)
	}
}

// La narración forzada tiene que llevarle al modelo los DATOS que juntó, o narra en
// el vacío. Van en el request final como texto plano (no como mensajes de rol "tool",
// que son justo lo que gatilla el 400).
func TestAnswerQuery_FinalNarration_CarriesToolResults(t *testing.T) {
	call := 0
	var finalMessages []loopMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		var req loopRequest
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		if call <= maxQueryIterations {
			w.Write([]byte(`{"choices":[{"message":{"content":null,"tool_calls":[
				{"id":"c","type":"function","function":{"name":"sum_movements","arguments":"{}"}}
			]}}]}`))
			return
		}
		finalMessages = req.Messages
		w.Write([]byte(`{"choices":[{"message":{"content":"listo","tool_calls":null}}]}`))
	}))
	defer server.Close()

	execute := func(name string, args json.RawMessage) (string, error) { return "total: 5000 ARS", nil }
	o := newQueryOrchestrator(server.URL)
	if _, err := o.AnswerQuery(context.Background(), "s", "cuánto gasté", nil,
		[]AgentTool{{Name: "sum_movements", Parameters: json.RawMessage(`{}`)}}, execute); err != nil {
		t.Fatalf("AnswerQuery: %v", err)
	}

	var blob string
	for _, m := range finalMessages {
		blob += m.Content + "\n"
	}
	if !strings.Contains(blob, "total: 5000 ARS") {
		t.Errorf("el request final tiene que llevar los resultados de las tools; mensajes = %+v", finalMessages)
	}
	if !strings.Contains(blob, "cuánto gasté") {
		t.Errorf("el request final tiene que conservar la pregunta original; mensajes = %+v", finalMessages)
	}
}

// Regresión del 2026-08-10: cuatro consultas seguidas murieron con 429 (TPM 8000,
// gpt-oss-120b) con el bucket LLENO — ningún otro tráfico en 4 horas. Groq cobra
// prompt + max_completion_tokens reservado por adelantado, así que cada llamada de
// query cuesta ~2.300 aunque narre 176 tokens. Con el cap de iteraciones en 3, una
// consulta podía hacer 4 llamadas: ~9.200 reservados, más que el techo entero.
//
// El techo real es este número de llamadas, no el cap de tokens: en 14 días de
// consultas exitosas, NINGUNA pasó de 3 llamadas. La 4ª solo existió para fallar.
// Este test falla si alguien vuelve a subir maxQueryIterations sin rehacer la cuenta.
func TestAnswerQuery_NeverExceedsThreeGroqCalls(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		// Nunca para de pedir tools: fuerza el peor caso.
		w.Write([]byte(`{"choices":[{"message":{"content":null,"tool_calls":[
			{"id":"c","type":"function","function":{"name":"sum_movements","arguments":"{}"}}
		]}}]}`))
	}))
	defer server.Close()

	execute := func(name string, args json.RawMessage) (string, error) { return "x", nil }
	o := newQueryOrchestrator(server.URL)
	o.AnswerQuery(context.Background(), "s", "u", nil,
		[]AgentTool{{Name: "sum_movements", Parameters: json.RawMessage(`{}`)}}, execute)

	if calls > 3 {
		t.Errorf("llamadas a Groq = %d, want <= 3: a ~2.300 reservados cada una, 4 no entran en el TPM de 8.000", calls)
	}
}

func TestAnswerQuery_PrependsHistory(t *testing.T) {
	var gotMessages []loopMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req loopRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotMessages = req.Messages
		// Return final content immediately (no tool calls) so the loop ends round 1.
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"listo","tool_calls":null}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, QueryModel: "m", TimeoutSeconds: 5})
	history := []QueryTurn{
		{Question: "cuanto gaste en automotor esta semana?", Answer: "0 ARS"},
	}
	_, err := o.AnswerQuery(context.Background(), "system", "y la semana anterior?", history,
		[]AgentTool{{Name: "sum_movements", Description: "d", Parameters: json.RawMessage(`{"type":"object"}`)}},
		func(name string, args json.RawMessage) (string, error) { return "", nil })
	if err != nil {
		t.Fatalf("AnswerQuery: %v", err)
	}

	// Expect: system, user(q1), assistant(a1), user(current)
	if len(gotMessages) != 4 {
		t.Fatalf("messages len = %d, want 4: %+v", len(gotMessages), gotMessages)
	}
	if gotMessages[0].Role != "system" {
		t.Errorf("msg[0] role = %q, want system", gotMessages[0].Role)
	}
	if gotMessages[1].Role != "user" || gotMessages[1].Content != "cuanto gaste en automotor esta semana?" {
		t.Errorf("msg[1] = %+v, want user q1", gotMessages[1])
	}
	if gotMessages[2].Role != "assistant" || gotMessages[2].Content != "0 ARS" {
		t.Errorf("msg[2] = %+v, want assistant a1", gotMessages[2])
	}
	if gotMessages[3].Role != "user" || gotMessages[3].Content != "y la semana anterior?" {
		t.Errorf("msg[3] = %+v, want user current", gotMessages[3])
	}
}

// Regresión del 2026-08-13, producción: el usuario preguntó por tres cosas
// ("cuánto gasté en disney, hbo y el lote"), el modelo alcanzó a consultar dos, y la
// narración forzada contestó "Disney + HBO: $9.990 / Lote: $8.122,73". El 8.122,73 es
// el total de HBO, puesto bajo la etiqueta del lote; del lote real ($30.343,74) no se
// consultó nada.
//
// No fue el modelo alucinando: `toolResults` juntaba SOLO el texto del resultado
// ("total: 9990.00 ARS", "total: 8122.73 ARS"), así que el request final llevaba dos
// números anónimos y una pregunta que nombraba tres cosas. Con eso, acertar la
// atribución es imposible.
//
// El camino normal nunca tuvo el problema: ahí cada resultado va como mensaje de rol
// "tool" con su ToolCallID, que sí lo asocia. Se perdía únicamente en toolResults, que
// es lo que alimenta la puerta 2 — por eso el fix es una línea ahí y no un eco en el
// ejecutor, que le cobraría tokens al 80% de las consultas que no lo necesitan.
func TestAnswerQuery_FinalNarration_PairsEachResultWithItsCall(t *testing.T) {
	call := 0
	var finalMessages []loopMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		var req loopRequest
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		// Una tool por ronda, con un filtro DISTINTO cada una: es el patrón real que
		// agota el cap y cae en la narración forzada.
		if call <= maxQueryIterations {
			filtro := "disney"
			if call == 2 {
				filtro = "hbo"
			}
			fmt.Fprintf(w, `{"choices":[{"message":{"content":null,"tool_calls":[
				{"id":"c%d","type":"function","function":{"name":"sum_movements","arguments":"{\"description\":\"%s\"}"}}
			]}}]}`, call, filtro)
			return
		}
		finalMessages = req.Messages
		w.Write([]byte(`{"choices":[{"message":{"content":"listo","tool_calls":null}}]}`))
	}))
	defer server.Close()

	// Montos distintos a propósito: con dos totales iguales, un intercambio de
	// etiquetas es indetectable y el test no probaría nada.
	execute := func(name string, args json.RawMessage) (string, error) {
		if strings.Contains(string(args), "hbo") {
			return "total: 8122.73 ARS", nil
		}
		return "total: 9990.00 ARS", nil
	}
	o := newQueryOrchestrator(server.URL)
	if _, err := o.AnswerQuery(context.Background(), "s", "cuánto gasté en disney, hbo y el lote", nil,
		[]AgentTool{{Name: "sum_movements", Parameters: json.RawMessage(`{}`)}}, execute); err != nil {
		t.Fatalf("AnswerQuery: %v", err)
	}

	var blob string
	for _, m := range finalMessages {
		blob += m.Content + "\n"
	}
	// Lo que importa no es que los números estén, sino que cada uno viaje pegado al
	// filtro que lo produjo. Se chequea por línea: el dato y su origen tienen que
	// llegar juntos, porque el modelo narra desde esto y nada más.
	for _, c := range []struct{ filtro, monto, ajeno string }{
		{"disney", "9990.00", "8122.73"},
		{"hbo", "8122.73", "9990.00"},
	} {
		var linea string
		for _, l := range strings.Split(blob, "\n") {
			// La línea del DATO, no la de la pregunta —que también nombra los tres
			// filtros—: la del dato es la que además lleva el nombre de la tool.
			if strings.Contains(l, c.filtro) && strings.Contains(l, "sum_movements") {
				linea = l
				break
			}
		}
		if linea == "" {
			t.Fatalf("ningún dato del request final está atribuido al filtro %q; blob:\n%s", c.filtro, blob)
		}
		if !strings.Contains(linea, c.monto) {
			t.Errorf("%q tiene que viajar con su total %s, y llegó como %q", c.filtro, c.monto, linea)
		}
		if strings.Contains(linea, c.ajeno) {
			t.Errorf("%q llegó con el total de OTRA llamada (%s): %q", c.filtro, c.ajeno, linea)
		}
	}
	// El nombre de la tool también: sin él, dos tools distintas con el mismo filtro
	// (sum_movements y list_movements sobre "hbo") vuelven a ser indistinguibles.
	if !strings.Contains(blob, "sum_movements") {
		t.Errorf("el request final tiene que nombrar la tool que produjo cada dato; blob:\n%s", blob)
	}
	// Y NADA de esto puede parecerse a una llamada a función. Ver el test de abajo.
	if strings.Contains(blob, `{"`) {
		t.Errorf("el request final no puede llevar JSON: el modelo lo imita y emite una tool call; blob:\n%s", blob)
	}
}

// Regresión del 2026-08-13, encontrada en el bot real y no por los tests: al hacer
// que cada resultado viajara con su llamada, la primera versión concatenaba el JSON
// CRUDO de los argumentos —`sum_movements {"currency":"ARS","description":"hbo"}`— y
// Groq devolvió 400 "Tool choice is none, but model called a tool". El
// failed_generation mostró al modelo imitando lo que veía:
//
//	{"name": "repo_browser.run_code", "arguments": {"tool": "sum_movements", ...}}
//
// TestAnswerQuery_FinalNarration_HistoryHasNoToolTrace ya cuidaba este mecanismo,
// pero por la puerta del HISTORIAL de mensajes. El patrón entró por el TEXTO, que
// ese test no mira — y por eso pasó verde mientras producción se caía.
//
// La regla es sobre la FORMA, no sobre el contenido: sin llaves, comillas ni
// paréntesis no hay sintaxis de llamada que imitar.
func TestDescribeCall_LooksNothingLikeAFunctionCall(t *testing.T) {
	got := describeCall("sum_movements", `{"currency":"ARS","description":"hbo","category":null,"from":"2026-08-01"}`)

	for _, prohibido := range []string{"{", "}", `"`, "(", ")"} {
		if strings.Contains(got, prohibido) {
			t.Errorf("describeCall no puede emitir %q —es sintaxis de llamada y el modelo la imita—: %s", prohibido, got)
		}
	}
	// Sigue identificando: nombre y filtros reales.
	for _, esperado := range []string{"sum_movements", "description=hbo", "currency=ARS", "from=2026-08-01"} {
		if !strings.Contains(got, esperado) {
			t.Errorf("falta %q en la descripción de la llamada: %s", esperado, got)
		}
	}
	// El null no viaja: el schema obliga al modelo a mandar los opcionales en null,
	// y "category=null" es ruido que invita a razonar sobre un filtro que nadie puso.
	if strings.Contains(got, "null") {
		t.Errorf("un argumento nulo no es un filtro y no tiene que aparecer: %s", got)
	}
	// Orden estable: dos resultados de la misma consulta tienen que leerse comparables.
	if got != describeCall("sum_movements", `{"from":"2026-08-01","description":"hbo","currency":"ARS","category":null}`) {
		t.Errorf("el mismo llamado con las claves en otro orden tiene que rendir igual: %s", got)
	}
}

// El loop de consultas no tenía cadena de fallback: iba directo contra queryModel y
// el primer 429 mataba el turno. El 2026-08-13 quedó en las trazas el caso que lo
// vuelve absurdo — un turno cuyo AGENTE fue rescatado (20b 429 → 120b 429 →
// llama-3.3-70b 200) murió un paso después, en la query, por no tener lo mismo que
// lo acababa de salvar.
func TestAnswerQuery_FallsBackToTheNextModelOnRateLimit(t *testing.T) {
	var usados []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req loopRequest
		json.NewDecoder(r.Body).Decode(&req)
		usados = append(usados, req.Model)
		if req.Model == "principal" {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":{"message":"Rate limit reached","code":"rate_limit_exceeded"}}`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"contestado","tool_calls":null}}]}`))
	}))
	defer server.Close()

	o := New(Config{
		APIKey: "k", BaseURL: server.URL, QueryModel: "principal",
		QueryFallbackModels: []string{"suplente"}, TimeoutSeconds: 5,
	})
	answer, err := o.AnswerQuery(context.Background(), "s", "u", nil,
		[]AgentTool{{Name: "sum_movements", Parameters: json.RawMessage(`{}`)}},
		func(string, json.RawMessage) (string, error) { return "ok", nil })
	if err != nil {
		t.Fatalf("AnswerQuery: %v", err)
	}
	if answer != "contestado" {
		t.Errorf("answer = %q, want la respuesta del suplente", answer)
	}
	// El principal va PRIMERO: si se invirtiera, el tráfico normal se iría al
	// suplente y la cadena dejaría de ser una red para pasar a ser el camino.
	if len(usados) < 2 || usados[0] != "principal" || usados[len(usados)-1] != "suplente" {
		t.Errorf("orden de modelos = %v, want principal y después suplente", usados)
	}
}

// Un 400 no se reintenta: es un error nuestro y sale igual en cualquier modelo.
// Reintentarlo gastaría el cupo de los suplentes para obtener el mismo error — y el
// cupo de los suplentes es justo lo que hay que tener guardado para el próximo 429.
func TestAnswerQuery_DoesNotFallBackOnABadRequest(t *testing.T) {
	var usados []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req loopRequest
		json.NewDecoder(r.Body).Decode(&req)
		usados = append(usados, req.Model)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"tool call validation failed","code":"tool_use_failed"}}`))
	}))
	defer server.Close()

	o := New(Config{
		APIKey: "k", BaseURL: server.URL, QueryModel: "principal",
		QueryFallbackModels: []string{"suplente"}, TimeoutSeconds: 5,
	})
	if _, err := o.AnswerQuery(context.Background(), "s", "u", nil,
		[]AgentTool{{Name: "sum_movements", Parameters: json.RawMessage(`{}`)}},
		func(string, json.RawMessage) (string, error) { return "ok", nil }); err == nil {
		t.Fatal("un 400 tiene que propagarse")
	}
	for _, m := range usados {
		if m == "suplente" {
			t.Errorf("se probó el suplente ante un 400: %v", usados)
		}
	}
}

// La narración forzada no decide nada: tiene los datos y sólo redacta. Corre en un
// modelo que NO razona, porque el modo de falla medido el 2026-08-13 es que el
// razonamiento se coma el presupuesto de completion y la respuesta vuelva vacía.
// Las rondas de tools se quedan donde estaban: ahí razonar sirve.
func TestAnswerQuery_ForcedNarrationUsesTheNarrationModel(t *testing.T) {
	var modelos []string
	call := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		var req loopRequest
		json.NewDecoder(r.Body).Decode(&req)
		modelos = append(modelos, req.Model)
		w.Header().Set("Content-Type", "application/json")
		if call <= maxQueryIterations {
			w.Write([]byte(`{"choices":[{"message":{"content":null,"tool_calls":[
				{"id":"c","type":"function","function":{"name":"sum_movements","arguments":"{}"}}
			]}}]}`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"listo","tool_calls":null}}]}`))
	}))
	defer server.Close()

	o := New(Config{
		APIKey: "k", BaseURL: server.URL,
		QueryModel: "razonador", NarrationModel: "redactor", TimeoutSeconds: 5,
	})
	if _, err := o.AnswerQuery(context.Background(), "s", "u", nil,
		[]AgentTool{{Name: "sum_movements", Parameters: json.RawMessage(`{}`)}},
		func(string, json.RawMessage) (string, error) { return "total: 1 ARS", nil }); err != nil {
		t.Fatalf("AnswerQuery: %v", err)
	}

	if len(modelos) != maxQueryIterations+1 {
		t.Fatalf("llamadas = %v, want %d rondas + 1 narración", modelos, maxQueryIterations)
	}
	for i := 0; i < maxQueryIterations; i++ {
		if modelos[i] != "razonador" {
			t.Errorf("ronda %d fue a %q, want el modelo de query", i, modelos[i])
		}
	}
	if got := modelos[len(modelos)-1]; got != "redactor" {
		t.Errorf("la narración fue a %q, want el modelo de narración", got)
	}
}

// Un entorno que no declare narrationModel no cambia de comportamiento: sigue
// narrando con el modelo de query, como antes de esta spec.
func TestAnswerQuery_NoNarrationModelKeepsUsingTheQueryModel(t *testing.T) {
	var modelos []string
	call := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		var req loopRequest
		json.NewDecoder(r.Body).Decode(&req)
		modelos = append(modelos, req.Model)
		w.Header().Set("Content-Type", "application/json")
		if call <= maxQueryIterations {
			w.Write([]byte(`{"choices":[{"message":{"content":null,"tool_calls":[
				{"id":"c","type":"function","function":{"name":"sum_movements","arguments":"{}"}}
			]}}]}`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"listo","tool_calls":null}}]}`))
	}))
	defer server.Close()

	o := New(Config{APIKey: "k", BaseURL: server.URL, QueryModel: "razonador", TimeoutSeconds: 5})
	if _, err := o.AnswerQuery(context.Background(), "s", "u", nil,
		[]AgentTool{{Name: "sum_movements", Parameters: json.RawMessage(`{}`)}},
		func(string, json.RawMessage) (string, error) { return "total: 1 ARS", nil }); err != nil {
		t.Fatalf("AnswerQuery: %v", err)
	}
	for i, m := range modelos {
		if m != "razonador" {
			t.Errorf("llamada %d fue a %q, want el modelo de query en todas", i, m)
		}
	}
}

// La cadena de narración no reintenta el mismo modelo dos veces: si el de narración
// ya está en la cadena de query, no se repite.
func TestNarrationChain_DoesNotRepeatAModel(t *testing.T) {
	o := New(Config{
		QueryModel:          "q",
		QueryFallbackModels: []string{"redactor", "z"},
		NarrationModel:      "redactor",
	})
	got := o.narrationChain()
	want := []string{"redactor", "q", "z"}
	if len(got) != len(want) {
		t.Fatalf("cadena = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("cadena = %v, want %v", got, want)
		}
	}
}

var _ = time.Second

// El cap de la narración es propio y más chico que el de las rondas. No es cosmético:
// Groq reserva prompt + max_completion_tokens contra el TPM AUNQUE no se usen, así que
// cada token de cap que no se necesita es cupo que se le saca a la consulta siguiente.
// Medido el 2026-08-13: narrar cuesta 35-61 tokens de completion en un modelo que no
// razona, así que 400 deja ~6 veces de margen.
func TestAnswerQuery_ForcedNarrationSendsItsOwnCompletionCap(t *testing.T) {
	var caps []int
	call := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		var req loopRequest
		json.NewDecoder(r.Body).Decode(&req)
		caps = append(caps, req.MaxCompletionTokens)
		w.Header().Set("Content-Type", "application/json")
		if call <= maxQueryIterations {
			w.Write([]byte(`{"choices":[{"message":{"content":null,"tool_calls":[
				{"id":"c","type":"function","function":{"name":"sum_movements","arguments":"{}"}}
			]}}]}`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"listo","tool_calls":null}}]}`))
	}))
	defer server.Close()

	o := New(Config{APIKey: "k", BaseURL: server.URL, QueryModel: "q", NarrationModel: "n", TimeoutSeconds: 5})
	if _, err := o.AnswerQuery(context.Background(), "s", "u", nil,
		[]AgentTool{{Name: "sum_movements", Parameters: json.RawMessage(`{}`)}},
		func(string, json.RawMessage) (string, error) { return "total: 1 ARS", nil }); err != nil {
		t.Fatalf("AnswerQuery: %v", err)
	}

	for i := 0; i < maxQueryIterations; i++ {
		if caps[i] != maxQueryCompletionTokens {
			t.Errorf("ronda %d mandó cap %d, want %d", i, caps[i], maxQueryCompletionTokens)
		}
	}
	if got := caps[len(caps)-1]; got != maxNarrationCompletionTokens {
		t.Errorf("la narración mandó cap %d, want %d", got, maxNarrationCompletionTokens)
	}
	if maxNarrationCompletionTokens >= maxQueryCompletionTokens {
		t.Errorf("el cap de narración (%d) tiene que ser MENOR que el de las rondas (%d): si no, no ahorra TPM reservado",
			maxNarrationCompletionTokens, maxQueryCompletionTokens)
	}
}
