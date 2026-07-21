//go:build llm_eval

package orchestrator

import (
	"context"
	"os"
	"testing"
)

// Run with real Groq creds:
//
//	GROQ_API_KEY=... GROQ_BASE_URL=... GROQ_CREATE_MODEL=... go test -tags llm_eval ./internal/orchestrator/ -run TestCreateEval -v
//
// Excluded from the default `go test ./...` (no tag) so CI needs no API key.
var evalCases = []struct {
	msg          string
	wantType     string // first movement's type
	wantLegs     int
	wantMerchant string
}{
	{"gasté 500 en el súper", "expense", 1, ""},
	{"me pagaron 10 mil", "income", 1, ""},
	{"transferí 100 mil a Pablo por la picada", "expense", 1, "Pablo"},
	{"di a Juan mil", "expense", 1, "Juan"},
	{"Emma me transfirió mil", "income", 1, "Emma"},
	{"compré 100 usd a 1500", "transfer", 2, ""},
	{"pasé 50 mil del banco a Mercado Pago", "transfer", 2, ""},
	{"pasé 10 mil a MP", "transfer", 2, ""}, // nickname match
	{"el broker rindió 10 mil", "income", 1, ""},
	{"compré pan, medicamentos y carne", "expense", 1, ""}, // 3 movements, first is expense
}

func evalTaxonomy() []TaxonomyEntry {
	return []TaxonomyEntry{
		{Category: "Alimentación", Subcategory: "Supermercado", Description: "Compras de supermercado"},
		{Category: "Ocio y salidas", Subcategory: "Restaurante", Description: "Comidas afuera, delivery, picadas"},
		{Category: "Sueldo", Subcategory: "Sueldo", Description: "Ingreso de sueldo"},
		{Category: "Inversiones", Subcategory: "Dólares", Description: "Compra/venta de dólares"},
		{Category: "Sistema", Subcategory: "Transferencia", Description: "Movimiento de dinero entre cuentas propias"},
		{Category: "Sistema", Subcategory: "Rendimiento inversión", Description: "Ganancia de una cuenta/inversión"},
		{Category: "Alimentación", Subcategory: "Almacén", Description: "Compras de almacén/kiosco/farmacia"},
	}
}

func evalAccounts() []AccountOption {
	return []AccountOption{
		{ID: 1, Name: "Banco", Currency: "ARS"},
		{ID: 2, Name: "Mercado Pago", Currency: "ARS"},
		{ID: 3, Name: "Broker", Currency: "USD"},
	}
}

func firstType(res CreateResult) string {
	if len(res.Movements) == 0 {
		return ""
	}
	return res.Movements[0].Type
}

func TestCreateEval(t *testing.T) {
	key := os.Getenv("GROQ_API_KEY")
	if key == "" {
		t.Skip("GROQ_API_KEY unset — real-LLM eval skipped")
	}
	o := New(Config{
		APIKey:         key,
		BaseURL:        os.Getenv("GROQ_BASE_URL"),
		CreateModel:    os.Getenv("GROQ_CREATE_MODEL"),
		TimeoutSeconds: 30,
	})
	for _, tc := range evalCases {
		t.Run(tc.msg, func(t *testing.T) {
			res, err := o.ClassifyCreate(context.Background(), tc.msg, evalTaxonomy(), evalAccounts(), "2026-07-07")
			if err != nil {
				t.Fatalf("ClassifyCreate: %v", err)
			}
			if len(res.Movements) < 1 || res.Movements[0].Type != tc.wantType {
				t.Errorf("%q → first type %v, want %v", tc.msg, firstType(res), tc.wantType)
			}
		})
	}
}
