package orchestrator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAgentPrompt_ContainsNumberFormatRule(t *testing.T) {
	prompt := BuildAgentPrompt("2026-08-12",
		[]AccountOption{{ID: 1, Name: "Mercado Pago", Currency: "ARS"}},
		nil, "", AgentTools(), "")

	if !strings.Contains(prompt, "REGLA DE FORMATO NUMÉRICO") {
		t.Error("el prompt del agente perdió la regla de formato numérico")
	}
	if strings.Contains(prompt, "%!") {
		t.Errorf("el prompt tiene un error de Sprintf: %q", prompt)
	}
}

func TestOnboardingPrompt_ContainsNumberFormatRule(t *testing.T) {
	var captured string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)
		captured = req.Messages[0].Content
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"accounts\":[]}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, CreateModel: "m", TimeoutSeconds: 5})
	_, _ = o.ClassifyOnboarding(context.Background(), "tengo 1000 en el banco")

	if !strings.Contains(captured, "REGLA DE FORMATO NUMÉRICO") {
		t.Errorf("onboarding prompt missing the number-format rule")
	}
}

func TestUpdatePrompt_ContainsNumberFormatRule(t *testing.T) {
	var captured string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)
		captured = req.Messages[0].Content
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"resolved\":false,\"movements\":[]}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, UpdateModel: "m", TimeoutSeconds: 5})
	_, _ = o.ResolveUpdate(context.Background(), "cambialo a 1.500", MovementCandidate{}, nil)

	if !strings.Contains(captured, "REGLA DE FORMATO NUMÉRICO") {
		t.Errorf("update prompt missing the number-format rule")
	}
}
