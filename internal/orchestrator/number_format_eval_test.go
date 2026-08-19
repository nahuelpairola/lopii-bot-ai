//go:build llm_eval

package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// Run with real Groq creds:
//   GROQ_APIKEY=... GROQ_BASE_URL=... GROQ_CREATE_MODEL=... go test -tags llm_eval ./internal/orchestrator/ -run TestNumberFormatEval -v
// Excluded from default `go test ./...` (build tag) so CI needs no API key.

// Las cuentas del eval. Vivían en create_eval_test.go, que se fue con
// ClassifyCreate; este es el único eval que las seguía usando.
var numberFormatEvalAccounts = []AccountOption{
	{ID: 1, Name: "Banco", Currency: "ARS"},
	{ID: 2, Name: "Mercado Pago", Currency: "ARS"},
	{ID: 3, Name: "Broker", Currency: "USD"},
}

var numberFormatCreateCases = []struct {
	msg        string
	wantAmount string // expected normalized amount on the first movement
}{
	{"gasté 1.041.265 en el súper", "1041265"},
	{"pagué 1.500,50 de luz", "1500.50"},
	{"salió 2.000 el café", "2000"},
	{"compré algo de 1.5m", "1500000"},
}

// El formato numérico ahora se prueba contra EL LOOP, que es quien extrae el
// monto desde que la etapa 5 borró ClassifyCreate. El sujeto no cambió y sigue
// siendo plata: leer "1.041.265" como 1041.265 cambia el monto MIL veces, y
// nada río abajo lo puede notar — es un número perfectamente válido.
func TestNumberFormatEval_Loop(t *testing.T) {
	key := evalKey(t)
	o := New(Config{
		APIKey:         key,
		BaseURL:        os.Getenv("GROQ_BASE_URL"),
		AgentModel:     os.Getenv("GROQ_AGENT_MODEL"),
		TimeoutSeconds: 30,
	})
	tools := AgentTools()
	prompt := BuildAgentPrompt("2026-07-07", numberFormatEvalAccounts, nil, "", tools, "")

	for _, tc := range numberFormatCreateCases {
		t.Run(tc.msg, func(t *testing.T) {
			var got string
			_, err := o.Run(context.Background(), prompt, tc.msg, nil, tools,
				func(name string, args json.RawMessage) (string, error) {
					if name != ToolRecordMovements {
						return "", fmt.Errorf("el loop llamó %s en vez de registrar", name)
					}
					var parsed struct {
						Movements []MovementDraft `json:"movements"`
					}
					if err := json.Unmarshal(args, &parsed); err != nil {
						return "", err
					}
					if len(parsed.Movements) > 0 {
						got = parsed.Movements[0].Amount
					}
					return "registrado", ErrAgentTurnDone
				})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if got != tc.wantAmount {
				t.Errorf("%q → amount %q, want %q", tc.msg, got, tc.wantAmount)
			}
		})
	}
}

var numberFormatOnboardingCases = []struct {
	msg         string
	wantBalance string // expected normalized balance on the first account
}{
	{"tengo 1.041.265 en el banco", "1041265"},
	{"en la caja de ahorro 1.500,50", "1500.50"},
	{"efectivo 2.000", "2000"},
}

func TestNumberFormatEval_Onboarding(t *testing.T) {
	key := evalKey(t)
	o := New(Config{
		APIKey:         key,
		BaseURL:        os.Getenv("GROQ_BASE_URL"),
		CreateModel:    os.Getenv("GROQ_CREATE_MODEL"), // ClassifyOnboarding uses createModel
		TimeoutSeconds: 30,
	})
	for _, tc := range numberFormatOnboardingCases {
		t.Run(tc.msg, func(t *testing.T) {
			res, err := o.ClassifyOnboarding(context.Background(), tc.msg)
			if err != nil {
				t.Fatalf("ClassifyOnboarding: %v", err)
			}
			if len(res.Accounts) < 1 {
				t.Fatalf("%q → no accounts", tc.msg)
			}
			if got := res.Accounts[0].Balance; got != tc.wantBalance {
				t.Errorf("%q → balance %q, want %q", tc.msg, got, tc.wantBalance)
			}
		})
	}
}
