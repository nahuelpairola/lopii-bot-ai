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

	client := NewClient("k", server.URL, 5*time.Second, nil)
	body, err := client.send(context.Background(), callTypeRouter, "m", []byte(`{}`))
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

func TestClient_Send_HonorsGroqBodyRetryAfterOnTPM429(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			// No Retry-After header — Groq doesn't set one for TPM (token-based)
			// 429s, only the free-text message says how long to wait.
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":{"message":"Rate limit reached for model x on tokens per minute (TPM): Limit 8000, Used 7696, Requested 2254. Please try again in 3.05s.","type":"tokens","code":"rate_limit_exceeded"}}`))
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := NewClient("k", server.URL, 10*time.Second, nil)
	start := time.Now()
	body, err := client.send(context.Background(), callTypeRouter, "m", []byte(`{}`))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("body = %s, want {\"ok\":true}", body)
	}
	if n := atomic.LoadInt32(&hits); n != 2 {
		t.Errorf("server hits = %d, want 2 (one 429 then success)", n)
	}
	// The old maxBackoff (1s) would cap this wait far short of the 3.05s Groq
	// asked for. Honoring the body means we actually wait close to it.
	if elapsed < 3*time.Second {
		t.Errorf("elapsed = %s, want >= 3s (should honor Groq's body-stated wait, not the old 1s blind cap)", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Errorf("elapsed = %s, want < 5s (shouldn't overshoot the stated wait by much)", elapsed)
	}
}

func TestClient_Send_NoRetryOnClientError(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadRequest) // 400 = our bug, must NOT retry
	}))
	defer server.Close()

	client := NewClient("k", server.URL, 5*time.Second, nil)
	if _, err := client.send(context.Background(), callTypeRouter, "m", []byte(`{}`)); err == nil {
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

	client := NewClient("test-key", server.URL, 5*time.Second, nil)
	raw, err := client.chatCompletion(context.Background(), callTypeRouter, "test-model", "system", "user message", toolSchema{
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

	client := NewClient("test-key", server.URL, 5*time.Second, nil)
	if _, err := client.chatCompletion(context.Background(), callTypeRouter, "m", "s", "u", toolSchema{Name: "x", Parameters: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("expected an error on a non-200 response")
	}
}

func TestClient_ChatCompletion_ErrorsOnMissingToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices": [{"message": {}}]}`))
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL, 5*time.Second, nil)
	if _, err := client.chatCompletion(context.Background(), callTypeRouter, "m", "s", "u", toolSchema{Name: "x", Parameters: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("expected an error when the response has no tool call")
	}
}

type fakeRecorder struct{ calls []LLMCall }

func (f *fakeRecorder) Record(c LLMCall) { f.calls = append(f.calls, c) }

func TestSendRecordsLLMCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-ratelimit-remaining-requests", "199")
		w.Header().Set("x-ratelimit-remaining-tokens", "48000")
		w.WriteHeader(200)
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{}"}}]}}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`))
	}))
	defer srv.Close()

	rec := &fakeRecorder{}
	c := NewClient("k", srv.URL, 5*time.Second, rec)
	ctx := WithTraceID(context.Background(), "trace-xyz")

	if _, err := c.send(ctx, callTypeRouter, "llama-3.1-8b-instant", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("want 1 record, got %d", len(rec.calls))
	}
	got := rec.calls[0]
	if got.TraceID != "trace-xyz" || got.CallType != callTypeRouter || got.Model != "llama-3.1-8b-instant" {
		t.Fatalf("bad ids: %+v", got)
	}
	if got.TotalTokens != 18 || got.HTTPStatus != 200 || got.Attempts != 1 {
		t.Fatalf("bad metrics: %+v", got)
	}
	if got.RateLimitRemainingRequests == nil || *got.RateLimitRemainingRequests != 199 {
		t.Fatalf("bad ratelimit: %+v", got)
	}
}

func TestSendRecordsAttemptsOnRetry(t *testing.T) {
	var n int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 1 {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{}"}}]}}]}`))
	}))
	defer srv.Close()
	rec := &fakeRecorder{}
	c := NewClient("k", srv.URL, 5*time.Second, rec)
	if _, err := c.send(context.Background(), callTypeCreate, "m", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if rec.calls[0].Attempts != 2 {
		t.Fatalf("want 2 attempts, got %d", rec.calls[0].Attempts)
	}
}
