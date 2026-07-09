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

func TestResolveDelete_AcceptsStringResolved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"resolved\":\"false\"}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, DeleteModel: "test-model", TimeoutSeconds: 5})
	candidate := MovementCandidate{Movements: []MovementDraft{{Description: "Nafta YPF"}}}

	result, err := o.ResolveDelete(context.Background(), "borrá lo de la nafta", candidate)
	if err != nil {
		t.Fatalf("ResolveDelete: %v", err)
	}
	if result.Resolved {
		t.Error("resolved = true, want false when the model sent it as the JSON string \"false\"")
	}
}

func TestResolveDelete_AcceptsNullMentionedDates(t *testing.T) {
	// Same schema-validation regression as update.go: Groq rejects the tool
	// call server-side if the model emits null for a field the schema
	// declares as plain "string".
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"resolved\":true,\"mentioned_date_from\":null,\"mentioned_date_to\":null}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, DeleteModel: "test-model", TimeoutSeconds: 5})
	candidate := MovementCandidate{Movements: []MovementDraft{{Description: "Nafta YPF"}}}

	result, err := o.ResolveDelete(context.Background(), "borrá lo de la nafta", candidate)
	if err != nil {
		t.Fatalf("ResolveDelete: %v", err)
	}
	if result.MentionedDateFrom != "" || result.MentionedDateTo != "" {
		t.Errorf("mentioned dates = %q/%q, want empty on null", result.MentionedDateFrom, result.MentionedDateTo)
	}
}

func TestResolveDelete_MentionedDateRange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"resolved\":false,\"mentioned_date_from\":\"2026-06-27\",\"mentioned_date_to\":\"2026-06-29\"}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, DeleteModel: "test-model", TimeoutSeconds: 5})
	candidate := MovementCandidate{Movements: []MovementDraft{{Description: "Nafta YPF"}}}

	result, err := o.ResolveDelete(context.Background(), "borrá lo de entre el 27 y el 29", candidate)
	if err != nil {
		t.Fatalf("ResolveDelete: %v", err)
	}
	if result.MentionedDateFrom != "2026-06-27" || result.MentionedDateTo != "2026-06-29" {
		t.Errorf("date range = %q..%q, want 2026-06-27..2026-06-29", result.MentionedDateFrom, result.MentionedDateTo)
	}
}
