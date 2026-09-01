//go:build llm_eval

package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

var numberFormatEvalAccounts = []AccountOption{
	{ID: 1, Name: "Banco", Currency: "ARS"},
	{ID: 2, Name: "Mercado Pago", Currency: "ARS"},
	{ID: 3, Name: "Broker", Currency: "USD"},
}

var numberFormatCreateCases = []struct {
	msg        string
	wantAmount string
}{
	{"gasté 1.041.265 en el súper", "1041265"},
	{"pagué 1.500,50 de luz", "1500.50"},
	{"salió 2.000 el café", "2000"},
	{"compré algo de 1.5m", "1500000"},
}

func TestNumberFormatEval_Loop(t *testing.T) {
	key := evalKey(t)
	o := New(Config{
		APIKey:         key,
		BaseURL:        evalBaseURL(),
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
	wantBalance string
}{
	{"tengo 1.041.265 en el banco", "1041265"},
	{"en la caja de ahorro 1.500,50", "1500.50"},
	{"efectivo 2.000", "2000"},
}

func TestNumberFormatEval_Onboarding(t *testing.T) {
	key := evalKey(t)
	o := New(Config{
		APIKey:         key,
		BaseURL:        evalBaseURL(),
		CreateModel:    os.Getenv("GROQ_CREATE_MODEL"),
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
