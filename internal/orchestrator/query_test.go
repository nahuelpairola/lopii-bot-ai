package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

// Regression: Groq hard-400s a request where tool_choice is "none" but the
// model still attempts a tool call — observed for real against gpt-oss-120b
// (not just the weak models), which broke the forced-narration guarantee the
// max-iterations path depends on. Root cause was sending toolDefs alongside
// tool_choice:"none"; the fix omits tools on that call so nothing exists for
// the model to call. This test pins the final round to always send zero
// tools, so a model attempting one is structurally impossible again.
func TestAnswerQuery_FinalNarration_NeverOffersTools(t *testing.T) {
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
		if len(req.Tools) != 0 {
			// Simulates the real Groq 400 this test guards against.
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":{"message":"Tool choice is none, but model called a tool"}}`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"Resumen sin herramientas.","tool_calls":null}}]}`))
	}))
	defer server.Close()

	execute := func(name string, args json.RawMessage) (string, error) { return "x", nil }
	o := newQueryOrchestrator(server.URL)
	answer, err := o.AnswerQuery(context.Background(), "s", "u", nil,
		[]AgentTool{{Name: "sum_movements", Parameters: json.RawMessage(`{}`)}}, execute)
	if err != nil {
		t.Fatalf("AnswerQuery: %v (final round must never offer tools, so the 400 must never happen)", err)
	}
	if answer != "Resumen sin herramientas." {
		t.Errorf("answer = %q", answer)
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

var _ = time.Second
