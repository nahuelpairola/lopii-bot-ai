package orchestrator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_Send_RetriesTransientThenSucceeds(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests) // transient — must retry
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := NewClient("k", server.URL, 5*time.Second)
	body, err := client.send(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("body = %s, want {\"ok\":true}", body)
	}
	if n := atomic.LoadInt32(&hits); n != 2 {
		t.Errorf("server hits = %d, want 2 (one 429 then success)", n)
	}
}

func TestClient_Send_NoRetryOnClientError(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadRequest) // 400 = our bug, must NOT retry
	}))
	defer server.Close()

	client := NewClient("k", server.URL, 5*time.Second)
	if _, err := client.send(context.Background(), []byte(`{}`)); err == nil {
		t.Fatal("expected an error on 400")
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Errorf("server hits = %d, want 1 (400 must not retry)", n)
	}
}

func TestClient_ChatCompletion_ReturnsToolArguments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization header = %q, want Bearer test-key", got)
		}
		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if req.Model != "test-model" {
			t.Errorf("request model = %q, want test-model", req.Model)
		}
		if req.ToolChoice.Function.Name != "do_thing" {
			t.Errorf("forced tool = %q, want do_thing", req.ToolChoice.Function.Name)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"choices": [{
				"message": {
					"tool_calls": [{
						"function": {"arguments": "{\"ok\":true}"}
					}]
				}
			}]
		}`))
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL, 5*time.Second)
	raw, err := client.chatCompletion(context.Background(), "test-model", "system", "user message", toolSchema{
		Name:       "do_thing",
		Parameters: json.RawMessage(`{"type":"object"}`),
	})
	if err != nil {
		t.Fatalf("chatCompletion: %v", err)
	}
	if string(raw) != `{"ok":true}` {
		t.Errorf("raw = %s, want {\"ok\":true}", raw)
	}
}

func TestClient_ChatCompletion_ErrorsOnNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest) // non-retryable 4xx → immediate error
		w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL, 5*time.Second)
	if _, err := client.chatCompletion(context.Background(), "m", "s", "u", toolSchema{Name: "x", Parameters: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("expected an error on a non-200 response")
	}
}

func TestClient_ChatCompletion_ErrorsOnMissingToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices": [{"message": {}}]}`))
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL, 5*time.Second)
	if _, err := client.chatCompletion(context.Background(), "m", "s", "u", toolSchema{Name: "x", Parameters: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("expected an error when the response has no tool call")
	}
}
