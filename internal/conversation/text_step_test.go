package conversation

import "testing"

func TestTextStep_EscapeOptionsFunc_RendersAndProcesses(t *testing.T) {
	step := TextStep{
		PromptText: func(Data) string { return "¿saldo?" },
		DataKey:    "balance",
		NextStep:   "confirm",
		EscapeOptionsFunc: func(data Data) []ChoiceOption {
			if data["balance"] == nil {
				return nil
			}
			return []ChoiceOption{{Label: "✅ Usar", Value: "confirm_seed", NextStep: "confirm"}}
		},
	}

	// Not seeded → no extra button.
	if got := step.Prompt(Data{}); len(got.Buttons) != 0 {
		t.Fatalf("unseeded prompt: got %d buttons, want 0", len(got.Buttons))
	}

	// Seeded → one confirm button.
	seeded := Data{"balance": "1000"}
	if got := step.Prompt(seeded); len(got.Buttons) != 1 || got.Buttons[0].Data != "confirm_seed" {
		t.Fatalf("seeded prompt buttons = %+v, want one confirm_seed", got.Buttons)
	}

	// Tapping the func-provided button advances to NextStep and keeps the seed.
	tr := step.Process(Input{CallbackData: "confirm_seed"}, seeded)
	if tr.kind != outcomeAdvance || tr.nextStep != "confirm" {
		t.Fatalf("process(confirm_seed) = %+v, want advance→confirm", tr)
	}
	if tr.data["balance"] != "1000" {
		t.Errorf("seed lost: balance = %v, want %q", tr.data["balance"], "1000")
	}
}
