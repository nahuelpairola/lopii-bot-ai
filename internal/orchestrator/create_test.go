package orchestrator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClassifyCreate_SingleMovement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)
		if !strings.Contains(req.Messages[0].Content, "Alimentación") {
			t.Error("expected the taxonomy block to be included in the system prompt")
		}
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"movements\":[{\"type\":\"expense\",\"amount\":\"3000\",\"currency\":\"ARS\",\"category\":\"Alimentación\",\"subcategory\":\"Café\",\"payment_method\":\"cash\",\"description\":\"Café\",\"date\":\"2026-07-02\"}]}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, CreateModel: "test-model", TimeoutSeconds: 5})
	taxonomy := []TaxonomyEntry{{Category: "Alimentación", Subcategory: "Café", Description: "Cafeterías"}}

	result, err := o.ClassifyCreate(context.Background(), "café 3000 efectivo", taxonomy, nil, "2026-07-02")
	if err != nil {
		t.Fatalf("ClassifyCreate: %v", err)
	}
	if len(result.Movements) != 1 {
		t.Fatalf("got %d movements, want 1", len(result.Movements))
	}
	if result.Movements[0].Amount != "3000" {
		t.Errorf("amount = %q, want 3000", result.Movements[0].Amount)
	}
}

func TestClassifyCreate_CompoundUSDPurchase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"movements\":[{\"type\":\"expense\",\"amount\":\"140000\",\"currency\":\"ARS\",\"category\":\"Inversiones\",\"subcategory\":\"Compra USD\",\"payment_method\":\"transfer\",\"description\":\"Compra 100 USD a 1400\",\"date\":\"2026-07-02\"},{\"type\":\"transfer\",\"amount\":\"100\",\"currency\":\"USD\",\"account_id\":5,\"category\":\"Inversiones\",\"subcategory\":\"Compra USD\",\"payment_method\":\"transfer\",\"description\":\"Compra 100 USD a 1400\",\"date\":\"2026-07-02\"}]}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, CreateModel: "test-model", TimeoutSeconds: 5})
	result, err := o.ClassifyCreate(context.Background(), "compré 100 usd a 1400", nil, nil, "2026-07-02")
	if err != nil {
		t.Fatalf("ClassifyCreate: %v", err)
	}
	if len(result.Movements) != 2 {
		t.Fatalf("got %d movements, want 2 (ARS expense + USD transfer)", len(result.Movements))
	}
	if result.Movements[1].AccountID == nil || *result.Movements[1].AccountID != 5 {
		t.Errorf("second movement account id = %v, want 5", result.Movements[1].AccountID)
	}
}

func TestClassifyCreate_ErrorsOnEmptyMovements(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"movements\":[]}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, CreateModel: "test-model", TimeoutSeconds: 5})
	if _, err := o.ClassifyCreate(context.Background(), "hola", nil, nil, "2026-07-02"); err == nil {
		t.Fatal("expected an error when the result has no movements")
	}
}
