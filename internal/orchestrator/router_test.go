package orchestrator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClassifyIntent_ReturnsCreate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"intent\":\"CREATE\"}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, RouterModel: "test-model", TimeoutSeconds: 5})
	intent, err := o.ClassifyIntent(context.Background(), "café 3000 efectivo")
	if err != nil {
		t.Fatalf("ClassifyIntent: %v", err)
	}
	if intent != IntentCreate {
		t.Errorf("intent = %q, want %q", intent, IntentCreate)
	}
}

func TestClassifyIntent_ErrorsOnUnknownIntent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"intent\":\"NONSENSE\"}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, RouterModel: "test-model", TimeoutSeconds: 5})
	if _, err := o.ClassifyIntent(context.Background(), "algo raro"); err == nil {
		t.Fatal("expected an error for an unrecognized intent value")
	}
}
