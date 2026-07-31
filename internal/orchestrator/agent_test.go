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

// agentToolsForTest is one tool of each class, so a test can assert ordering.
func agentToolsForTest() []AgentTool {
	return []AgentTool{
		{Name: "sum_movements", Kind: KindRead, Parameters: json.RawMessage(`{"type":"object"}`)},
		{Name: "record_movements", Kind: KindWrite, Parameters: json.RawMessage(`{"type":"object"}`)},
		{Name: "correct_movement", Kind: KindAction, Parameters: json.RawMessage(`{"type":"object"}`)},
	}
}

// loopServer replies with the given canned response bodies, one per request,
// and records every request it received.
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
	// The turn cut: content alongside tool_calls means the model already
	// narrated. Execute the calls, send the narration, spend no extra round.
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
	// tool_calls with no content: the model has not narrated, so the loop
	// must feed the results back and go again.
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
	// The tool result must be fed back with its call id.
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

func TestRun_RunsWritesBeforeReadsRegardlessOfModelOrder(t *testing.T) {
	// §4.1.2: the model lists the read FIRST. If the loop honoured that, the
	// sum would exclude the movements record_movements is about to insert —
	// wrong money, silently.
	srv, _ := loopServer(t, `{"choices":[{"message":{"content":"listo","tool_calls":[
		{"id":"c1","type":"function","function":{"name":"sum_movements","arguments":"{}"}},
		{"id":"c2","type":"function","function":{"name":"correct_movement","arguments":"{}"}},
		{"id":"c3","type":"function","function":{"name":"record_movements","arguments":"{}"}}]}}]}`)
	defer srv.Close()
	o := New(Config{BaseURL: srv.URL, AgentModel: "m", TimeoutSeconds: 5})

	var ran []string
	if _, err := o.Run(context.Background(), "sys", "x", nil, agentToolsForTest(),
		func(name string, _ json.RawMessage) (string, error) {
			ran = append(ran, name)
			return "ok", nil
		}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []string{"record_movements", "sum_movements", "correct_movement"}
	if strings.Join(ran, ",") != strings.Join(want, ",") {
		t.Errorf("ran %v, want %v (write, then read, then action)", ran, want)
	}
}

func TestRun_ToolErrorIsFedBackNotFatal(t *testing.T) {
	// A failing executor must reach the model as text so it can recover or
	// explain, exactly as AnswerQuery does today.
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
	// tool_choice:"required" on round 0 — without it the model can reply
	// "listo, anoté tus $5.000" having called nothing: silent data loss.
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
