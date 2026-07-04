package orchestrator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveUpdate_Resolved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"resolved\":true,\"movements\":[{\"type\":\"expense\",\"amount\":\"150\",\"currency\":\"USD\",\"category\":\"Otros\",\"subcategory\":\"Otros\",\"payment_method\":\"transfer\",\"description\":\"Café\",\"date\":\"2026-07-02\"}]}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, UpdateModel: "test-model", TimeoutSeconds: 5})
	candidate := MovementCandidate{TransactionID: "", Movements: []MovementDraft{{Amount: "100", Currency: "USD"}}}

	result, err := o.ResolveUpdate(context.Background(), "en realidad fueron 150 usd", candidate)
	if err != nil {
		t.Fatalf("ResolveUpdate: %v", err)
	}
	if !result.Resolved {
		t.Error("expected resolved=true")
	}
	if len(result.Movements) != 1 || result.Movements[0].Amount != "150" {
		t.Errorf("movements = %+v, want amount 150", result.Movements)
	}
}

func TestResolveUpdate_Unresolved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"resolved\":false,\"movements\":[]}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, UpdateModel: "test-model", TimeoutSeconds: 5})
	candidate := MovementCandidate{Movements: []MovementDraft{{Amount: "100", Currency: "USD", Description: "Nafta"}}}

	result, err := o.ResolveUpdate(context.Background(), "el café de ayer era 3000", candidate)
	if err != nil {
		t.Fatalf("ResolveUpdate: %v", err)
	}
	if result.Resolved {
		t.Error("expected resolved=false")
	}
}

func TestResolveUpdate_MentionedDateRange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"resolved\":false,\"mentioned_date_from\":\"2026-06-27\",\"mentioned_date_to\":\"2026-06-29\",\"movements\":[]}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, UpdateModel: "test-model", TimeoutSeconds: 5})
	candidate := MovementCandidate{Movements: []MovementDraft{{Amount: "100", Currency: "USD"}}}

	result, err := o.ResolveUpdate(context.Background(), "fue entre el 27 y el 29", candidate)
	if err != nil {
		t.Fatalf("ResolveUpdate: %v", err)
	}
	if result.MentionedDateFrom != "2026-06-27" || result.MentionedDateTo != "2026-06-29" {
		t.Errorf("date range = %q..%q, want 2026-06-27..2026-06-29", result.MentionedDateFrom, result.MentionedDateTo)
	}
}
