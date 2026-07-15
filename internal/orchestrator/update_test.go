package orchestrator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildUpdateSystemPrompt_IncludesAccountsAndRule(t *testing.T) {
	accs := []AccountOption{
		{ID: 7, Name: "Banco Galicia", Currency: "ARS"},
		{ID: 9, Name: "FCI", Currency: "ARS"},
	}
	prompt := buildUpdateSystemPrompt(accs)

	for _, want := range []string{"Banco Galicia", "FCI", "7 | Banco Galicia (ARS)"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	// The account-correction rule must be present so the model knows to remap.
	if !strings.Contains(strings.ToLower(prompt), "account_id") || !strings.Contains(prompt, "corrige la cuenta") {
		t.Errorf("prompt missing the account-correction rule:\n%s", prompt)
	}
}

func TestResolveUpdate_Resolved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"resolved\":true,\"movements\":[{\"type\":\"expense\",\"amount\":\"150\",\"currency\":\"USD\",\"category\":\"Otros\",\"subcategory\":\"Otros\",\"payment_method\":\"transfer\",\"description\":\"Café\",\"date\":\"2026-07-02\"}]}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, UpdateModel: "test-model", TimeoutSeconds: 5})
	candidate := MovementCandidate{TransactionID: "", Movements: []MovementDraft{{Amount: "100", Currency: "USD"}}}

	result, err := o.ResolveUpdate(context.Background(), "en realidad fueron 150 usd", candidate, nil)
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

	result, err := o.ResolveUpdate(context.Background(), "el café de ayer era 3000", candidate, nil)
	if err != nil {
		t.Fatalf("ResolveUpdate: %v", err)
	}
	if result.Resolved {
		t.Error("expected resolved=false")
	}
}

func TestResolveUpdate_AcceptsStringResolved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"resolved\":\"true\",\"movements\":[]}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, UpdateModel: "test-model", TimeoutSeconds: 5})
	candidate := MovementCandidate{Movements: []MovementDraft{{Amount: "100", Currency: "USD"}}}

	result, err := o.ResolveUpdate(context.Background(), "en realidad fueron 150 usd", candidate, nil)
	if err != nil {
		t.Fatalf("ResolveUpdate: %v", err)
	}
	if !result.Resolved {
		t.Error("resolved = false, want true when the model sent it as the JSON string \"true\"")
	}
}

func TestResolveUpdate_AcceptsNullMentionedDates(t *testing.T) {
	// Groq validates tool-call arguments against our JSON schema server-side
	// (see flexBool comment) — a model that emits null for an unmentioned
	// date field 400s unless the schema allows it. Regression for the
	// "Pablo me devolvió cien pesos por el asado" 400 (no date in message).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"resolved\":true,\"mentioned_date_from\":null,\"mentioned_date_to\":null,\"movements\":[{\"type\":\"expense\",\"amount\":\"4900\",\"currency\":\"ARS\",\"category\":\"Alimentación\",\"subcategory\":\"Almacén / barrio\",\"payment_method\":\"transfer\",\"description\":\"pago a pablo por el asado\",\"date\":\"2026-07-07\"}]}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, UpdateModel: "test-model", TimeoutSeconds: 5})
	candidate := MovementCandidate{Movements: []MovementDraft{{Amount: "5000", Currency: "ARS", Description: "pago a pablo por el asado"}}}

	result, err := o.ResolveUpdate(context.Background(), "Pablo me devolvió cien pesos por el asado", candidate, nil)
	if err != nil {
		t.Fatalf("ResolveUpdate: %v", err)
	}
	if result.MentionedDateFrom != "" || result.MentionedDateTo != "" {
		t.Errorf("mentioned dates = %q/%q, want empty on null", result.MentionedDateFrom, result.MentionedDateTo)
	}
}

func TestResolveUpdate_NetsReintegro(t *testing.T) {
	// Regression for the netting rule in updateSystemPrompt: a reintegro
	// discounts off the ORIGINAL amount, never lands as its own income.
	// Canonical worked example from the prompt itself: café 700, reintegro
	// 100 -> 600.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"resolved\":true,\"movements\":[{\"type\":\"expense\",\"amount\":\"600\",\"currency\":\"ARS\",\"category\":\"Alimentación\",\"subcategory\":\"Salir a comer\",\"payment_method\":\"transfer\",\"description\":\"café\",\"date\":\"2026-07-10\"}]}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, UpdateModel: "test-model", TimeoutSeconds: 5})
	candidate := MovementCandidate{Movements: []MovementDraft{{Amount: "700", Currency: "ARS", Description: "café"}}}

	result, err := o.ResolveUpdate(context.Background(), "me devolvieron 100 del café", candidate, nil)
	if err != nil {
		t.Fatalf("ResolveUpdate: %v", err)
	}
	if !result.Resolved {
		t.Fatal("expected resolved=true")
	}
	if len(result.Movements) != 1 || result.Movements[0].Amount != "600" {
		t.Errorf("movements = %+v, want single movement netted to amount 600 (700 - 100)", result.Movements)
	}
	if result.Movements[0].Type != "expense" {
		t.Errorf("type = %q, want expense — a reintegro nets the original, it never becomes income", result.Movements[0].Type)
	}
}

func TestResolveUpdate_MentionedDateRange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"resolved\":false,\"mentioned_date_from\":\"2026-06-27\",\"mentioned_date_to\":\"2026-06-29\",\"movements\":[]}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, UpdateModel: "test-model", TimeoutSeconds: 5})
	candidate := MovementCandidate{Movements: []MovementDraft{{Amount: "100", Currency: "USD"}}}

	result, err := o.ResolveUpdate(context.Background(), "fue entre el 27 y el 29", candidate, nil)
	if err != nil {
		t.Fatalf("ResolveUpdate: %v", err)
	}
	if result.MentionedDateFrom != "2026-06-27" || result.MentionedDateTo != "2026-06-29" {
		t.Errorf("date range = %q..%q, want 2026-06-27..2026-06-29", result.MentionedDateFrom, result.MentionedDateTo)
	}
}
