package orchestrator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// capturingRecorder guarda el último LLMCall emitido. Record corre desde send()
// de forma síncrona, pero el mutex evita depender de eso.
type capturingRecorder struct {
	mu   sync.Mutex
	last LLMCall
}

func (r *capturingRecorder) Record(c LLMCall) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.last = c
}

func (r *capturingRecorder) get() LLMCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last
}

func groqStub(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// El corpus del eval de la etapa 5 promete medir "selección Y argumentos". Sin
// esto llm_calls sólo puede reconstruir la selección, y la mitad de argumentos
// vuelve a casos inventados a mano — justo lo que el corpus real reemplaza.
func TestRecord_CapturesToolCallArguments(t *testing.T) {
	rec := &capturingRecorder{}
	srv := groqStub(t, `{"choices":[{"message":{"tool_calls":[
		{"id":"c1","type":"function","function":{"name":"record_movements","arguments":"{\"movements\":[]}"}}]}}],
		"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)

	c := NewClient("k", srv.URL, 5*time.Second, rec)
	if _, err := c.send(context.Background(), "agent", "m", []byte(`{}`)); err != nil {
		t.Fatalf("send: %v", err)
	}

	got := rec.get().ToolCalls
	if !strings.Contains(got, "record_movements") {
		t.Errorf("ToolCalls = %q, want la tool registrada", got)
	}
	if !strings.Contains(got, "movements") {
		t.Errorf("ToolCalls = %q, want también los argumentos", got)
	}
}

// Vacío y no "[]": server lo mapea a NULL, así que `WHERE tool_calls IS NOT
// NULL` significa "el modelo llamó algo".
func TestRecord_NoToolCallsLeavesTheFieldEmpty(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"narración sin tools", `{"choices":[{"message":{"content":"hola"}}]}`},
		{"array vacío", `{"choices":[{"message":{"tool_calls":[]}}]}`},
		{"tool_calls null", `{"choices":[{"message":{"tool_calls":null}}]}`},
		{"sin choices", `{"usage":{"prompt_tokens":1}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &capturingRecorder{}
			srv := groqStub(t, tc.body)

			c := NewClient("k", srv.URL, 5*time.Second, rec)
			if _, err := c.send(context.Background(), "agent", "m", []byte(`{}`)); err != nil {
				t.Fatalf("send: %v", err)
			}

			if got := rec.get().ToolCalls; got != "" {
				t.Errorf("ToolCalls = %q, want vacío", got)
			}
		})
	}
}
