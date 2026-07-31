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

// TestTextStep_OnTextTransformsDataAfterTheAnswer covers what ask_user's
// self-looping step needs: a text answer that is consumed, not just stored.
// Without the hook every round overwrites DataKey and only the last answer
// survives, so a flow asking two questions loses the first.
func TestTextStep_OnTextTransformsDataAfterTheAnswer(t *testing.T) {
	step := TextStep{
		PromptText: func(Data) string { return "¿en qué categoría?" },
		DataKey:    "answer",
		NextStep:   "ask",
		OnText: func(text string, data Data) Data {
			next := Data{}
			for k, v := range data {
				next[k] = v
			}
			// What ask_user really does: file the answer against the question
			// it belongs to, and drop the raw scratch key.
			next["answered_"+stringOrEmptyForTest(data["pending"])] = text
			delete(next, "answer")
			return next
		},
	}

	tr := step.Process(Input{Text: "Comida"}, Data{"pending": "categoria"})

	if got := tr.data["answered_categoria"]; got != "Comida" {
		t.Errorf("answered_categoria = %v, want Comida", got)
	}
	if _, still := tr.data["answer"]; still {
		t.Error("OnText must be able to drop the raw DataKey it received")
	}
	if tr.nextStep != "ask" {
		t.Errorf("nextStep = %q, want ask (the self-loop)", tr.nextStep)
	}
}

// TestTextStep_NoOnTextKeepsTodaysBehaviour is the guard: every existing
// TextStep leaves OnText nil and must be unaffected.
func TestTextStep_NoOnTextKeepsTodaysBehaviour(t *testing.T) {
	step := TextStep{
		PromptText: func(Data) string { return "¿nombre?" },
		DataKey:    "name",
		NextStep:   "next",
	}

	tr := step.Process(Input{Text: "Brubank"}, Data{})

	if tr.data["name"] != "Brubank" {
		t.Errorf("name = %v, want Brubank", tr.data["name"])
	}
	if tr.nextStep != "next" {
		t.Errorf("nextStep = %q, want next", tr.nextStep)
	}
}

func stringOrEmptyForTest(v any) string {
	s, _ := v.(string)
	return s
}
