package orchestrator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreatePrompt_ContainsNumberFormatRule(t *testing.T) {
	var captured string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)
		captured = req.Messages[0].Content
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"movements\":[{\"type\":\"expense\",\"amount\":\"500\",\"currency\":\"ARS\",\"category\":\"x\",\"subcategory\":\"y\",\"payment_method\":\"cash\",\"description\":\"z\",\"date\":\"2026-07-07\"}]}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, CreateModel: "m", TimeoutSeconds: 5})
	_, _ = o.ClassifyCreate(context.Background(), "gasté 500", nil, nil, "2026-07-07")

	if !strings.Contains(captured, "REGLA DE FORMATO NUMÉRICO") {
		t.Errorf("create prompt missing the number-format rule")
	}
	if strings.Contains(captured, "%!") {
		t.Errorf("create prompt has a Sprintf format error: %q", captured)
	}
}
