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

func TestClassifyCreate_PromptIncludesTransferAndFXPatterns(t *testing.T) {
	var captured string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)
		captured = req.Messages[0].Content
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"movements\":[{\"type\":\"expense\",\"amount\":\"1\",\"currency\":\"ARS\",\"category\":\"Otros\",\"subcategory\":\"Otros\",\"payment_method\":\"cash\",\"description\":\"x\",\"date\":\"2026-07-07\"}]}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, CreateModel: "test-model", TimeoutSeconds: 5})
	if _, err := o.ClassifyCreate(context.Background(), "pasé 50 mil del banco a mp", nil, nil, "2026-07-07"); err != nil {
		t.Fatalf("ClassifyCreate: %v", err)
	}

	for _, want := range []string{
		"Transferencia entre cuentas propias",
		"Sistema | Transferencia",
		"Inversiones | Dólares",
		"Compra/venta de USD",
	} {
		if !strings.Contains(captured, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
}

func TestClassifyCreate_CompoundUSDPurchase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"{\"movements\":[{\"type\":\"transfer\",\"amount\":\"-150000\",\"currency\":\"ARS\",\"account_id\":3,\"category\":\"Inversiones\",\"subcategory\":\"Dólares\",\"payment_method\":\"transfer\",\"description\":\"Compra 100 USD a 1500\",\"date\":\"2026-07-07\"},{\"type\":\"transfer\",\"amount\":\"100\",\"currency\":\"USD\",\"account_id\":5,\"category\":\"Inversiones\",\"subcategory\":\"Dólares\",\"payment_method\":\"transfer\",\"description\":\"Compra 100 USD a 1500\",\"date\":\"2026-07-07\"}]}"}}]}}]}`))
	}))
	defer server.Close()

	o := New(Config{BaseURL: server.URL, CreateModel: "test-model", TimeoutSeconds: 5})
	result, err := o.ClassifyCreate(context.Background(), "compré 100 usd a 1500", nil, nil, "2026-07-07")
	if err != nil {
		t.Fatalf("ClassifyCreate: %v", err)
	}
	if len(result.Movements) != 2 {
		t.Fatalf("got %d movements, want 2 (ARS transfer + USD transfer)", len(result.Movements))
	}
	for i, m := range result.Movements {
		if m.Type != "transfer" {
			t.Errorf("movement %d type = %q, want transfer (no expense leg anymore)", i, m.Type)
		}
		if m.AccountID == nil {
			t.Errorf("movement %d has nil account_id, want both legs attributed", i)
		}
		if m.Subcategory != "Dólares" {
			t.Errorf("movement %d subcategory = %q, want Dólares", i, m.Subcategory)
		}
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
