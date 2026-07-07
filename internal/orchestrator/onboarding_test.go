package orchestrator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClassifyOnboarding_MultipleAccounts(t *testing.T) {
	var captured string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)
		captured = req.Messages[0].Content
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"accounts\":[{\"name\":\"Banco\",\"currency\":\"ARS\",\"balance\":\"20000\"},{\"name\":\"Efectivo\",\"currency\":\"ARS\",\"balance\":\"5000\"},{\"name\":\"Bróker\",\"currency\":\"USD\",\"balance\":\"100\"}]}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, CreateModel: "test-model", TimeoutSeconds: 5})
	result, err := o.ClassifyOnboarding(context.Background(), "20k en el banco, 5k efectivo, 100 usd en el bróker")
	if err != nil {
		t.Fatalf("ClassifyOnboarding: %v", err)
	}
	if len(result.Accounts) != 3 {
		t.Fatalf("got %d accounts, want 3", len(result.Accounts))
	}
	if result.Accounts[2].Currency != "USD" || result.Accounts[2].Balance != "100" {
		t.Errorf("third account = %+v, want USD/100", result.Accounts[2])
	}
	// The prompt must forbid unit/asset positions and instruct ARS default.
	for _, want := range []string{"NUNCA unidades", "Efectivo", "ARS"} {
		if !strings.Contains(captured, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
}

func TestClassifyOnboarding_EmptyIsValid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"accounts\":[]}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, CreateModel: "test-model", TimeoutSeconds: 5})
	result, err := o.ClassifyOnboarding(context.Background(), "hola")
	if err != nil {
		t.Fatalf("ClassifyOnboarding: %v", err)
	}
	if len(result.Accounts) != 0 {
		t.Fatalf("got %d accounts, want 0 (empty is valid, not an error)", len(result.Accounts))
	}
}
