package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var errBoom = errors.New("boom")

func agentToolsForTest() []AgentTool {
	return []AgentTool{
		{Name: "sum_movements", Kind: KindRead, Parameters: json.RawMessage(`{"type":"object"}`)},
		{Name: "record_movements", Kind: KindWrite, Parameters: json.RawMessage(`{"type":"object"}`)},
		{Name: "correct_movement", Kind: KindAction, Parameters: json.RawMessage(`{"type":"object"}`)},
	}
}

func loopServer(t *testing.T, bodies ...string) (*httptest.Server, *[]loopRequest) {
	t.Helper()
	var got []loopRequest
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req loopRequest
		json.NewDecoder(r.Body).Decode(&req)
		got = append(got, req)
		if n >= len(bodies) {
			t.Errorf("unexpected extra round %d", n+1)
			w.Write([]byte(`{"choices":[{"message":{"content":"fin"}}]}`))
			return
		}
		w.Write([]byte(bodies[n]))
		n++
	}))
	return srv, &got
}

func TestRun_CutsWhenAssistantSendsContentWithToolCalls(t *testing.T) {
	srv, reqs := loopServer(t, `{"choices":[{"message":{"content":"Listo, anoté 2.","tool_calls":[
		{"id":"c1","type":"function","function":{"name":"record_movements","arguments":"{}"}}]}}]}`)
	defer srv.Close()
	o := New(Config{BaseURL: srv.URL, AgentModel: "m", TimeoutSeconds: 5})

	var ran []string
	out, err := o.Run(context.Background(), "sys", "gasté 2 lucas", nil, agentToolsForTest(),
		func(name string, _ json.RawMessage) (string, error) {
			ran = append(ran, name)
			return "ok", nil
		})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "Listo, anoté 2." {
		t.Errorf("narration = %q, want the assistant's content", out)
	}
	if len(ran) != 1 || ran[0] != "record_movements" {
		t.Errorf("executed %v, want the write to still run", ran)
	}
	if len(*reqs) != 1 {
		t.Errorf("%d rounds, want 1 — the cut must not spend a narration round", len(*reqs))
	}
}

func TestRun_NoContentMeansAnotherRound(t *testing.T) {
	srv, reqs := loopServer(t,
		`{"choices":[{"message":{"tool_calls":[
			{"id":"c1","type":"function","function":{"name":"sum_movements","arguments":"{}"}}]}}]}`,
		`{"choices":[{"message":{"content":"Gastaste 5.000."}}]}`)
	defer srv.Close()
	o := New(Config{BaseURL: srv.URL, AgentModel: "m", TimeoutSeconds: 5})

	out, err := o.Run(context.Background(), "sys", "cuánto gasté", nil, agentToolsForTest(),
		func(string, json.RawMessage) (string, error) { return "5000", nil })
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "Gastaste 5.000." {
		t.Errorf("narration = %q", out)
	}
	if len(*reqs) != 2 {
		t.Fatalf("%d rounds, want 2", len(*reqs))
	}
	var sawToolResult bool
	for _, m := range (*reqs)[1].Messages {
		if m.Role == "tool" && m.ToolCallID == "c1" && m.Content == "5000" {
			sawToolResult = true
		}
	}
	if !sawToolResult {
		t.Error("the tool result must be fed back as role:tool with its tool_call_id")
	}
}

func TestRun_TurnDoneEndsTheTurnWithoutANarrationRound(t *testing.T) {
	srv, reqs := loopServer(t, `{"choices":[{"message":{"tool_calls":[
		{"id":"c1","type":"function","function":{"name":"record_movements","arguments":"{}"}},
		{"id":"c2","type":"function","function":{"name":"correct_movement","arguments":"{}"}}]}}]}`)
	defer srv.Close()
	o := New(Config{BaseURL: srv.URL, AgentModel: "m", TimeoutSeconds: 5})

	var ran []string
	out, err := o.Run(context.Background(), "sys", "la panadería eran 2 mil", nil, agentToolsForTest(),
		func(name string, _ json.RawMessage) (string, error) {
			ran = append(ran, name)
			if name == "correct_movement" {
				return "parkeada", ErrAgentTurnDone
			}
			return "ok", nil
		})
	if err != nil {
		t.Fatalf("Run: %v — ErrAgentTurnDone no es un error, es el fin del turno", err)
	}
	if out != "" {
		t.Errorf("narration = %q, want empty — la copy la escribe la app", out)
	}
	if len(ran) != 2 {
		t.Errorf("executed %v, want both calls of the round", ran)
	}
	if len(*reqs) != 1 {
		t.Errorf("%d rounds, want 1 — la segunda vuelta es la que revienta el TPM", len(*reqs))
	}
}

func TestRun_ToolErrorIsFedBackNotFatal(t *testing.T) {
	srv, reqs := loopServer(t,
		`{"choices":[{"message":{"tool_calls":[
			{"id":"c1","type":"function","function":{"name":"sum_movements","arguments":"{}"}}]}}]}`,
		`{"choices":[{"message":{"content":"No pude leer eso."}}]}`)
	defer srv.Close()
	o := New(Config{BaseURL: srv.URL, AgentModel: "m", TimeoutSeconds: 5})

	if _, err := o.Run(context.Background(), "sys", "x", nil, agentToolsForTest(),
		func(string, json.RawMessage) (string, error) { return "", errBoom }); err != nil {
		t.Fatalf("Run must not abort on a tool error: %v", err)
	}
	for _, m := range (*reqs)[1].Messages {
		if m.Role == "tool" && strings.Contains(m.Content, "boom") {
			return
		}
	}
	t.Error("the executor error must be fed back as a tool message")
}

func TestRun_FirstRoundForcesAToolCall(t *testing.T) {
	srv, reqs := loopServer(t, `{"choices":[{"message":{"content":"ok","tool_calls":[
		{"id":"c1","type":"function","function":{"name":"sum_movements","arguments":"{}"}}]}}]}`)
	defer srv.Close()
	o := New(Config{BaseURL: srv.URL, AgentModel: "m", TimeoutSeconds: 5})

	if _, err := o.Run(context.Background(), "sys", "x", nil, agentToolsForTest(),
		func(string, json.RawMessage) (string, error) { return "ok", nil }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if (*reqs)[0].ToolChoice != "required" {
		t.Errorf("round 0 tool_choice = %q, want required", (*reqs)[0].ToolChoice)
	}
}

func TestRun_HistoryIsReplayedBeforeTheUserMessage(t *testing.T) {
	srv, reqs := loopServer(t, `{"choices":[{"message":{"content":"ok","tool_calls":[
		{"id":"c1","type":"function","function":{"name":"sum_movements","arguments":"{}"}}]}}]}`)
	defer srv.Close()
	o := New(Config{BaseURL: srv.URL, AgentModel: "m", TimeoutSeconds: 5})

	history := []QueryTurn{{Question: "cuánto gasté", Answer: "5.000"}}
	if _, err := o.Run(context.Background(), "sys", "y en dólares?", history, agentToolsForTest(),
		func(string, json.RawMessage) (string, error) { return "ok", nil }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	msgs := (*reqs)[0].Messages
	if len(msgs) != 4 ||
		msgs[0].Role != "system" ||
		msgs[1].Content != "cuánto gasté" ||
		msgs[2].Content != "5.000" ||
		msgs[3].Content != "y en dólares?" {
		t.Errorf("messages = %+v, want system, prior Q, prior A, current user", msgs)
	}
}

func TestRun_ToolUseFailedFallsBackToAuto(t *testing.T) {
	var choices []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req loopRequest
		json.NewDecoder(r.Body).Decode(&req)
		choices = append(choices, req.ToolChoice)
		if req.ToolChoice == "required" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":{"message":"Tool choice is required, but model did not call a tool","type":"invalid_request_error","code":"tool_use_failed","failed_generation":""}}`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"¿De qué movimiento hablás?"}}]}`))
	}))
	defer srv.Close()
	o := New(Config{BaseURL: srv.URL, AgentModel: "m", TimeoutSeconds: 5})

	out, err := o.Run(context.Background(), "sys", "Le erre eran 1500", nil, agentToolsForTest(),
		func(string, json.RawMessage) (string, error) { return "ok", nil })
	if err != nil {
		t.Fatalf("Run must recover from tool_use_failed, got: %v", err)
	}
	if out != "¿De qué movimiento hablás?" {
		t.Errorf("narration = %q, want the clarifying question", out)
	}
	if len(choices) < 2 || choices[0] != "required" || choices[1] != "auto" {
		t.Errorf("tool_choice sequence = %v, want [required auto]", choices)
	}
}

func TestCompletionCapsAreSeparate(t *testing.T) {
	if maxQueryCompletionTokens != 1024 {
		t.Errorf("el cap de AnswerQuery = %d, want 1024: es el camino vivo y no se toca en esta etapa", maxQueryCompletionTokens)
	}
	if maxAgentCompletionTokens <= maxQueryCompletionTokens {
		t.Errorf("el cap del loop (%d) tiene que ser mayor que el de query (%d): una sola respuesta puede traer todas las tool_calls de la ronda MÁS la narración",
			maxAgentCompletionTokens, maxQueryCompletionTokens)
	}
}

func TestRun_SendsItsOwnCompletionCap(t *testing.T) {
	srv, reqs := loopServer(t, `{"choices":[{"message":{"content":"ok","tool_calls":[
		{"id":"c1","type":"function","function":{"name":"sum_movements","arguments":"{}"}}]}}]}`)
	defer srv.Close()
	o := New(Config{BaseURL: srv.URL, AgentModel: "m", TimeoutSeconds: 5})

	if _, err := o.Run(context.Background(), "sys", "x", nil, agentToolsForTest(),
		func(string, json.RawMessage) (string, error) { return "ok", nil }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := (*reqs)[0].MaxCompletionTokens; got != maxAgentCompletionTokens {
		t.Errorf("max_completion_tokens = %d, want %d", got, maxAgentCompletionTokens)
	}
}

func TestAgentRound_FallsBackToTheNextModelOnRateLimit(t *testing.T) {
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
		w.Write([]byte(`{"choices":[{"message":{"content":"listo"}}]}`))
	}))
	defer server.Close()

	o := New(Config{
		BaseURL: server.URL, AgentModel: "principal",
		AgentFallbackModels: []string{"suplente"}, TimeoutSeconds: 5,
	})
	msg, err := o.agentRound(context.Background(), []loopMessage{{Role: "user", Content: "hola"}}, nil, "auto")
	if err != nil {
		t.Fatalf("agentRound: %v", err)
	}
	if msg.Content != "listo" {
		t.Errorf("content = %q, want la respuesta del suplente", msg.Content)
	}
	if len(usados) < 2 || usados[0] != "principal" || usados[len(usados)-1] != "suplente" {
		t.Errorf("orden de modelos = %v, want principal y después suplente", usados)
	}
}

func TestAgentRound_DoesNotFallBackOnABadRequest(t *testing.T) {
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
		BaseURL: server.URL, AgentModel: "principal",
		AgentFallbackModels: []string{"suplente"}, TimeoutSeconds: 5,
	})
	if _, err := o.agentRound(context.Background(), []loopMessage{{Role: "user", Content: "hola"}}, nil, "auto"); err == nil {
		t.Fatal("un 400 tiene que propagarse")
	}
	for _, m := range usados {
		if m == "suplente" {
			t.Errorf("se probó el suplente ante un 400: %v", usados)
		}
	}
}
