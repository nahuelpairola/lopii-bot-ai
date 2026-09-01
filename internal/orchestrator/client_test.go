package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"lopiibot.com/internal/trace"
)

func TestParseBodyRetryAfter_MinutesAndSeconds(t *testing.T) {
	body := []byte(`{"error":{"message":"Rate limit reached. Please try again in 16m29.28s."}}`)
	got := parseBodyRetryAfter(body)
	want := 16*time.Minute + 29280*time.Millisecond
	if got < want-time.Second || got > want+time.Second {
		t.Fatalf("got %v, want ~%v", got, want)
	}
}

func TestParseBodyRetryAfter_SecondsOnly(t *testing.T) {
	body := []byte(`{"error":{"message":"try again in 4.185s"}}`)
	if got := parseBodyRetryAfter(body); got < 4*time.Second || got > 5*time.Second {
		t.Fatalf("got %v, want ~4.185s", got)
	}
}

func TestParseResetTokens(t *testing.T) {
	h := http.Header{}
	h.Set("x-ratelimit-reset-tokens", "7.66s")
	if got := parseResetTokens(h); got < 7*time.Second || got > 8*time.Second {
		t.Fatalf("got %v, want ~7.66s", got)
	}
}

func TestSend_TerminalRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-ratelimit-reset-tokens", "12s")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit reached. Please try again in 12s."}}`))
	}))
	defer srv.Close()

	c := NewClient("k", srv.URL, 5*time.Second, nil)
	_, err := c.send(context.Background(), "router", "m", []byte(`{}`))

	var rl *RateLimitedError
	if !errors.As(err, &rl) {
		t.Fatalf("want RateLimitedError, got %v", err)
	}
	if rl.RetryAfter < 10*time.Second || rl.RetryAfter > 13*time.Second {
		t.Fatalf("RetryAfter = %v, want ~12s", rl.RetryAfter)
	}
}

func TestClient_Send_RetriesTransientThenSucceeds(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
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
		t.Errorf("server hits = %d, want 2 (one 5xx then success)", n)
	}
}

func TestClient_Send_TPM429FailsFastAndSurfacesGroqWait(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit reached for model x on tokens per minute (TPM): Limit 8000, Used 7696, Requested 2254. Please try again in 3.05s.","type":"tokens","code":"rate_limit_exceeded"}}`))
	}))
	defer server.Close()

	client := NewClient("k", server.URL, 10*time.Second, nil)
	start := time.Now()
	_, err := client.send(context.Background(), callTypeRouter, "m", []byte(`{}`))
	elapsed := time.Since(start)

	var rl *RateLimitedError
	if !errors.As(err, &rl) {
		t.Fatalf("want a RateLimitedError, got %v", err)
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Errorf("server hits = %d, want 1 (a 429 must not retry)", n)
	}
	if want := 3050 * time.Millisecond; rl.RetryAfter != want {
		t.Errorf("RetryAfter = %s, want %s (el wait que dicta el body de Groq)", rl.RetryAfter, want)
	}
	if elapsed > time.Second {
		t.Errorf("elapsed = %s, want < 1s (no se duerme el wait adentro del webhook)", elapsed)
	}
}

func TestClient_Send_NoRetryOnClientError(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadRequest)
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

func TestClient_Send_CreateRetriesOnceOnToolUseFailedThenSucceeds(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":{"message":"Tool choice is required, but model did not call a tool","code":"tool_use_failed"}}`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{}"}}]}}]}`))
	}))
	defer server.Close()

	client := NewClient("k", server.URL, 5*time.Second, nil)
	if _, err := client.send(context.Background(), callTypeCreate, "m", []byte(`{}`)); err != nil {
		t.Fatalf("send: %v", err)
	}
	if n := atomic.LoadInt32(&hits); n != 2 {
		t.Errorf("server hits = %d, want 2 (one tool_use_failed then success)", n)
	}
}

func TestClient_Send_CreateGivesUpAfterOneRetryOnPersistentToolUseFailed(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"Tool choice is required, but model did not call a tool","code":"tool_use_failed"}}`))
	}))
	defer server.Close()

	client := NewClient("k", server.URL, 5*time.Second, nil)
	_, err := client.send(context.Background(), callTypeCreate, "m", []byte(`{}`))
	if !errors.Is(err, ErrNothingToExtract) {
		t.Fatalf("want ErrNothingToExtract, got %v", err)
	}
	if n := atomic.LoadInt32(&hits); n != 2 {
		t.Errorf("server hits = %d, want 2 (1 retry, then give up — not the full maxSendAttempts)", n)
	}
}

func TestClient_Send_NonCreateDoesNotRetryOnToolUseFailed(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"Tool choice is required, but model did not call a tool","code":"tool_use_failed"}}`))
	}))
	defer server.Close()

	client := NewClient("k", server.URL, 5*time.Second, nil)
	_, err := client.send(context.Background(), callTypeRouter, "m", []byte(`{}`))
	if !errors.Is(err, ErrNothingToExtract) {
		t.Fatalf("want ErrNothingToExtract, got %v", err)
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Errorf("server hits = %d, want 1 (retry is create-only)", n)
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
		w.WriteHeader(http.StatusBadRequest)
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
	ctx := trace.WithID(context.Background(), "trace-xyz")

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
