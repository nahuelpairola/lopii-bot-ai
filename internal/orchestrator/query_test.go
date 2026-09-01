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

func TestAnswerQuery_NeverExceedsTheRoundBudget(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":null,"tool_calls":[
			{"id":"c","type":"function","function":{"name":"sum_movements","arguments":"{}"}}
		]}}]}`))
	}))
	defer server.Close()

	execute := func(name string, args json.RawMessage) (string, error) { return "x", nil }
	o := newQueryOrchestrator(server.URL)
	o.AnswerQuery(context.Background(), "s", "u", nil,
		[]AgentTool{{Name: "sum_movements", Parameters: json.RawMessage(`{}`)}}, execute)

	if want := maxQueryIterations + 1; calls > want {
		t.Errorf("llamadas a Groq = %d, want <= %d (%d rondas + la narración forzada)", calls, want, maxQueryIterations)
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

func TestAnswerQuery_FinalNarration_PairsEachResultWithItsCall(t *testing.T) {
	call := 0
	var finalMessages []loopMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		var req loopRequest
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
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
	for _, c := range []struct{ filtro, monto, ajeno string }{
		{"disney", "9990.00", "8122.73"},
		{"hbo", "8122.73", "9990.00"},
	} {
		var linea string
		for _, l := range strings.Split(blob, "\n") {
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
	if !strings.Contains(blob, "sum_movements") {
		t.Errorf("el request final tiene que nombrar la tool que produjo cada dato; blob:\n%s", blob)
	}
	if strings.Contains(blob, `{"`) {
		t.Errorf("el request final no puede llevar JSON: el modelo lo imita y emite una tool call; blob:\n%s", blob)
	}
}

func TestDescribeCall_LooksNothingLikeAFunctionCall(t *testing.T) {
	got := describeCall("sum_movements", `{"currency":"ARS","description":"hbo","category":null,"from":"2026-08-01"}`)

	for _, prohibido := range []string{"{", "}", `"`, "(", ")"} {
		if strings.Contains(got, prohibido) {
			t.Errorf("describeCall no puede emitir %q —es sintaxis de llamada y el modelo la imita—: %s", prohibido, got)
		}
	}
	for _, esperado := range []string{"sum_movements", "description=hbo", "currency=ARS", "from=2026-08-01"} {
		if !strings.Contains(got, esperado) {
			t.Errorf("falta %q en la descripción de la llamada: %s", esperado, got)
		}
	}
	if strings.Contains(got, "null") {
		t.Errorf("un argumento nulo no es un filtro y no tiene que aparecer: %s", got)
	}
	if got != describeCall("sum_movements", `{"from":"2026-08-01","description":"hbo","currency":"ARS","category":null}`) {
		t.Errorf("el mismo llamado con las claves en otro orden tiene que rendir igual: %s", got)
	}
}

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
	if len(usados) < 2 || usados[0] != "principal" || usados[len(usados)-1] != "suplente" {
		t.Errorf("orden de modelos = %v, want principal y después suplente", usados)
	}
}

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

	if got := caps[0]; got != maxFirstRoundCompletionTokens {
		t.Errorf("la ronda 0 mandó cap %d, want %d", got, maxFirstRoundCompletionTokens)
	}
	for i := 1; i < maxQueryIterations; i++ {
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
	if maxFirstRoundCompletionTokens >= maxQueryCompletionTokens {
		t.Errorf("el cap de la ronda 0 (%d) tiene que ser MENOR que el de las rondas que narran (%d): si no, no ahorra nada",
			maxFirstRoundCompletionTokens, maxQueryCompletionTokens)
	}
}

func TestAnswerQuery_RunsEveryToolCallInOneRound(t *testing.T) {
	var ejecutadas []string
	call := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		w.Header().Set("Content-Type", "application/json")
		if call == 1 {
			w.Write([]byte(`{"choices":[{"message":{"content":null,"tool_calls":[
				{"id":"a","type":"function","function":{"name":"sum_movements","arguments":"{\"search\":\"Peaje\"}"}},
				{"id":"b","type":"function","function":{"name":"sum_movements","arguments":"{\"search\":\"Gas\"}"}},
				{"id":"c","type":"function","function":{"name":"sum_movements","arguments":"{\"search\":\"Cuota\"}"}}
			]}}]}`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"listo","tool_calls":null}}]}`))
	}))
	defer server.Close()

	o := newQueryOrchestrator(server.URL)
	if _, err := o.AnswerQuery(context.Background(), "s", "u", nil,
		[]AgentTool{{Name: "sum_movements", Parameters: json.RawMessage(`{}`)}},
		func(name string, args json.RawMessage) (string, error) {
			ejecutadas = append(ejecutadas, string(args))
			return "total: 1 ARS", nil
		}); err != nil {
		t.Fatalf("AnswerQuery: %v", err)
	}

	if len(ejecutadas) != 3 {
		t.Fatalf("la ronda traía 3 tool calls y se ejecutaron %d: %v", len(ejecutadas), ejecutadas)
	}
}
