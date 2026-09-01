package orchestrator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestChatCompletionLoop_ReturnsToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req loopRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.ToolChoice != "auto" {
			t.Errorf("tool_choice = %q, want auto", req.ToolChoice)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":null,"tool_calls":[
			{"id":"call_1","type":"function","function":{"name":"sum_movements","arguments":"{\"currency\":\"ARS\"}"}}
		]}}]}`))
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL, 5*time.Second, nil)
	msg, err := client.chatCompletionLoop(context.Background(), callTypeQuery, "test-model",
		[]loopMessage{{Role: "user", Content: "cuánto gasté"}}, nil, "auto", maxQueryCompletionTokens)
	if err != nil {
		t.Fatalf("chatCompletionLoop: %v", err)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "sum_movements" {
		t.Fatalf("tool calls = %+v, want one sum_movements call", msg.ToolCalls)
	}
	if msg.ToolCalls[0].ID != "call_1" {
		t.Errorf("tool call id = %q, want call_1", msg.ToolCalls[0].ID)
	}
}

func TestChatCompletionLoop_ReturnsContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"Gastaste 5000 ARS.","tool_calls":null}}]}`))
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL, 5*time.Second, nil)
	msg, err := client.chatCompletionLoop(context.Background(), callTypeQuery, "m", []loopMessage{{Role: "user", Content: "x"}}, nil, "auto", maxQueryCompletionTokens)
	if err != nil {
		t.Fatalf("chatCompletionLoop: %v", err)
	}
	if len(msg.ToolCalls) != 0 {
		t.Errorf("tool calls = %+v, want none", msg.ToolCalls)
	}
	if msg.Content != "Gastaste 5000 ARS." {
		t.Errorf("content = %q", msg.Content)
	}
}

func TestChatCompletionLoop_ErrorsOnNonOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL, 5*time.Second, nil)
	if _, err := client.chatCompletionLoop(context.Background(), callTypeQuery, "m", []loopMessage{{Role: "user", Content: "x"}}, nil, "auto", maxQueryCompletionTokens); err == nil {
		t.Fatal("expected an error on a non-200 response")
	}
}
