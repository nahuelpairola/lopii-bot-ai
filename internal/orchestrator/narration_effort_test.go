package orchestrator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func narrationEfforts(t *testing.T, queryModel string) (efforts []string, choices []string) {
	t.Helper()
	call := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		var req loopRequest
		json.NewDecoder(r.Body).Decode(&req)
		efforts = append(efforts, req.ReasoningEffort)
		choices = append(choices, req.ToolChoice)
		w.Header().Set("Content-Type", "application/json")
		if call <= maxQueryIterations {
			w.Write([]byte(`{"choices":[{"message":{"content":null,"tool_calls":[
				{"id":"c","type":"function","function":{"name":"sum_movements","arguments":"{}"}}
			]}}]}`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"Resumen.","tool_calls":null}}]}`))
	}))
	defer server.Close()

	o := New(Config{APIKey: "k", BaseURL: server.URL, QueryModel: queryModel, TimeoutSeconds: 5})
	execute := func(name string, args json.RawMessage) (string, error) { return "total: 5000 ARS", nil }
	if _, err := o.AnswerQuery(context.Background(), "s", "u", nil,
		[]AgentTool{{Name: "sum_movements", Parameters: json.RawMessage(`{}`)}}, execute); err != nil {
		t.Fatalf("AnswerQuery: %v", err)
	}
	return efforts, choices
}

func TestForcedNarration_AsksForLowReasoning(t *testing.T) {
	efforts, choices := narrationEfforts(t, "openai/gpt-oss-20b")

	last := len(efforts) - 1
	if choices[last] != "none" {
		t.Fatalf("la última llamada tiene tool_choice %q, want none — no es la narración forzada", choices[last])
	}
	if efforts[last] != "low" {
		t.Errorf("narración forzada con reasoning_effort %q, want low: sin esto el razonamiento se come el cap de %d y la respuesta vuelve vacía", efforts[last], maxNarrationCompletionTokens)
	}
	for i := 0; i < last; i++ {
		if efforts[i] != "" {
			t.Errorf("ronda %d mandó reasoning_effort %q: las rondas eligen tools y ahí el razonamiento sirve", i, efforts[i])
		}
	}
}

func TestForcedNarration_OmitsEffortForModelsThatRejectIt(t *testing.T) {
	efforts, _ := narrationEfforts(t, "qwen/qwen3.6-27b")

	if got := efforts[len(efforts)-1]; got != "" {
		t.Errorf("narración forzada con reasoning_effort %q sobre qwen, want vacío: Groq lo rechaza con un 400 que nadie reintenta", got)
	}
}
