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
	_, _ = o.ResolveUpdate(context.Background(), "cambialo a 1.500", MovementCandidate{})

	if !strings.Contains(captured, "REGLA DE FORMATO NUMÉRICO") {
		t.Errorf("update prompt missing the number-format rule")
	}
}
