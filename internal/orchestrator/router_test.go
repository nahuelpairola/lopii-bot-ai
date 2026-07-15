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
	result, err := o.ClassifyIntent(context.Background(), "café 3000 efectivo")
	if err != nil {
		t.Fatalf("ClassifyIntent: %v", err)
	}
	if result.Intent != IntentCreate {
		t.Errorf("intent = %q, want %q", result.Intent, IntentCreate)
	}
	if result.NeedsConfirmation {
		t.Error("needs_confirmation = true, want false when the model didn't set it")
	}
}

func TestClassifyIntent_ReturnsNeedsConfirmation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"intent\":\"CREATE\",\"needs_confirmation\":true}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, RouterModel: "test-model", TimeoutSeconds: 5})
	result, err := o.ClassifyIntent(context.Background(), "20k")
	if err != nil {
		t.Fatalf("ClassifyIntent: %v", err)
	}
	if !result.NeedsConfirmation {
		t.Error("needs_confirmation = false, want true when the model flagged it")
	}
}

func TestClassifyIntent_AcceptsStringNeedsConfirmation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"intent\":\"CREATE\",\"needs_confirmation\":\"true\"}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, RouterModel: "test-model", TimeoutSeconds: 5})
	result, err := o.ClassifyIntent(context.Background(), "20k")
	if err != nil {
		t.Fatalf("ClassifyIntent: %v", err)
	}
	if !result.NeedsConfirmation {
		t.Error("needs_confirmation = false, want true when the model sent it as the JSON string \"true\"")
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

func TestClassifyIntent_ReturnsAccountCreate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"intent\":\"ACCOUNT_MANAGE\"}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, RouterModel: "test-model", TimeoutSeconds: 5})
	result, err := o.ClassifyIntent(context.Background(), "quiero crear una cuenta nueva")
	if err != nil {
		t.Fatalf("ClassifyIntent: %v", err)
	}
	if result.Intent != IntentAccountManage {
		t.Errorf("intent = %q, want %q", result.Intent, IntentAccountManage)
	}
}

func TestClassifyIntent_ReturnsCreateCategory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"intent\":\"CREATE_CATEGORY\"}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, RouterModel: "test-model", TimeoutSeconds: 5})
	result, err := o.ClassifyIntent(context.Background(), "quiero crear una categoría nueva")
	if err != nil {
		t.Fatalf("ClassifyIntent: %v", err)
	}
	if result.Intent != IntentCreateCategory {
		t.Errorf("intent = %q, want %q", result.Intent, IntentCreateCategory)
	}
}
