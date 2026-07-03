package orchestrator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveDelete_Resolved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"resolved\":true}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, DeleteModel: "test-model", TimeoutSeconds: 5})
	candidate := MovementCandidate{Movements: []MovementDraft{{Description: "Nafta YPF"}}}

	result, err := o.ResolveDelete(context.Background(), "borrá lo de la nafta", candidate)
	if err != nil {
		t.Fatalf("ResolveDelete: %v", err)
	}
	if !result.Resolved {
		t.Error("expected resolved=true")
	}
}

func TestResolveDelete_Unresolved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"resolved\":false}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, DeleteModel: "test-model", TimeoutSeconds: 5})
	candidate := MovementCandidate{Movements: []MovementDraft{{Description: "Sueldo"}}}

	result, err := o.ResolveDelete(context.Background(), "borrá lo de la nafta", candidate)
	if err != nil {
		t.Fatalf("ResolveDelete: %v", err)
	}
	if result.Resolved {
		t.Error("expected resolved=false")
	}
}
