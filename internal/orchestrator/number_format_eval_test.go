//go:build llm_eval

package orchestrator

import (
	"context"
	"os"
	"testing"
)

// Run with real Groq creds:
//   GROQ_API_KEY=... GROQ_BASE_URL=... GROQ_CREATE_MODEL=... go test -tags llm_eval ./internal/orchestrator/ -run TestNumberFormatEval -v
// Excluded from default `go test ./...` (build tag) so CI needs no API key.

var numberFormatCreateCases = []struct {
	msg        string
	wantAmount string // expected normalized amount on the first movement
}{
	{"gasté 1.041.265 en el súper", "1041265"},
	{"pagué 1.500,50 de luz", "1500.50"},
	{"salió 2.000 el café", "2000"},
	{"compré algo de 1.5m", "1500000"},
}

func TestNumberFormatEval_Create(t *testing.T) {
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
	for _, tc := range numberFormatCreateCases {
		t.Run(tc.msg, func(t *testing.T) {
			res, err := o.ClassifyCreate(context.Background(), tc.msg, evalTaxonomy(), evalAccounts(), "2026-07-07")
			if err != nil {
				t.Fatalf("ClassifyCreate: %v", err)
			}
			if len(res.Movements) < 1 {
				t.Fatalf("%q → no movements", tc.msg)
			}
			if got := res.Movements[0].Amount; got != tc.wantAmount {
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
	key := os.Getenv("GROQ_API_KEY")
	if key == "" {
		t.Skip("GROQ_API_KEY unset — real-LLM eval skipped")
	}
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
