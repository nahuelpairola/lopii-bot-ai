package conversation

import "testing"

func TestTextStep_Skip_NilSkipIf(t *testing.T) {
	step := TextStep{DataKey: "amount", NextStep: "next"}
	if _, ok := step.Skip(Data{"amount": "100"}); ok {
		t.Error("Skip should be false when SkipIf is nil")
	}
}

func TestTextStep_Skip_DelegatesToSkipIf(t *testing.T) {
	step := TextStep{
		SkipIf: func(data Data) (string, bool) {
			return "elsewhere", true
		},
	}
	next, ok := step.Skip(Data{})
	if !ok || next != "elsewhere" {
		t.Errorf("Skip = %q, %v; want %q, true", next, ok, "elsewhere")
	}
}

func TestChoiceStep_Skip_NilSkipIf(t *testing.T) {
	step := ChoiceStep{}
	if _, ok := step.Skip(Data{}); ok {
		t.Error("Skip should be false when SkipIf is nil")
	}
}

func TestChoiceStep_Skip_DelegatesToSkipIf(t *testing.T) {
	step := ChoiceStep{
		SkipIf: func(data Data) (string, bool) {
			return "elsewhere", true
		},
	}
	next, ok := step.Skip(Data{})
	if !ok || next != "elsewhere" {
		t.Errorf("Skip = %q, %v; want %q, true", next, ok, "elsewhere")
	}
}

// fakeStep is a minimal Step used to test the engine/flow skip
// mechanism directly, without going through TextStep/ChoiceStep.
type fakeStep struct {
	nextStep string
	skip     func(data Data) (string, bool)
	// process, if set, overrides the default Process behavior (plain
	// Advance(nextStep, data) with data untouched). Lets a specific test
	// simulate a step that actually writes to data before advancing.
	process func(input Input, data Data) Transition
}

func (s fakeStep) Prompt(data Data) Prompt { return Prompt{Text: "prompt:" + s.nextStep} }
func (s fakeStep) Process(input Input, data Data) Transition {
	if s.process != nil {
		return s.process(input, data)
	}
	return Advance(s.nextStep, data)
}
func (s fakeStep) PossibleNextSteps() []string { return []string{s.nextStep} }
func (s fakeStep) Skip(data Data) (string, bool) {
	if s.skip == nil {
		return "", false
	}
	return s.skip(data)
}

func TestFlow_AdvanceThroughSkips_StopsAtNonSkippableStep(t *testing.T) {
	steps := map[string]Step{
		"a": fakeStep{nextStep: "b", skip: func(Data) (string, bool) { return "b", true }},
		"b": fakeStep{nextStep: "c"}, // not skippable
		"c": fakeStep{nextStep: "c"},
	}
	flow, err := NewFlow("test", "a", steps)
	if err != nil {
		t.Fatalf("NewFlow: %v", err)
	}

	resolved, err := flow.advanceThroughSkips("a", Data{})
	if err != nil {
		t.Fatalf("advanceThroughSkips: %v", err)
	}
	if resolved != "b" {
		t.Errorf("resolved = %q, want %q", resolved, "b")
	}
}

func TestFlow_AdvanceThroughSkips_EmptyNextStepMeansComplete(t *testing.T) {
	steps := map[string]Step{
		"a": fakeStep{nextStep: "b", skip: func(Data) (string, bool) { return "", true }},
		"b": fakeStep{nextStep: "b"},
	}
	flow, err := NewFlow("test", "a", steps)
	if err != nil {
		t.Fatalf("NewFlow: %v", err)
	}

	resolved, err := flow.advanceThroughSkips("a", Data{})
	if err != nil {
		t.Fatalf("advanceThroughSkips: %v", err)
	}
	if resolved != "" {
		t.Errorf("resolved = %q, want empty (complete)", resolved)
	}
}

func TestFlow_AdvanceThroughSkips_CycleGuard(t *testing.T) {
	steps := map[string]Step{
		"a": fakeStep{nextStep: "b", skip: func(Data) (string, bool) { return "b", true }},
		"b": fakeStep{nextStep: "a", skip: func(Data) (string, bool) { return "a", true }},
	}
	flow, err := NewFlow("test", "a", steps)
	if err != nil {
		t.Fatalf("NewFlow: %v", err)
	}

	if _, err := flow.advanceThroughSkips("a", Data{}); err == nil {
		t.Fatal("expected an error from an infinite Skip loop, got nil")
	}
}

func TestTextStep_Process_EscapeOption_Advances(t *testing.T) {
	step := TextStep{
		DataKey:  "name",
		NextStep: "next",
		EscapeOptions: []ChoiceOption{
			{Label: "Atrás", Value: "back", NextStep: "previous"},
		},
	}
	transition := step.Process(Input{CallbackData: "back"}, Data{"untouched": "yes"})
	if transition.kind != outcomeAdvance || transition.nextStep != "previous" {
		t.Fatalf("transition = %+v, want Advance to %q", transition, "previous")
	}
	if transition.data["untouched"] != "yes" {
		t.Errorf("data should pass through unchanged when OnEscape is nil, got %+v", transition.data)
	}
}

func TestTextStep_Process_EscapeOption_Finish(t *testing.T) {
	step := TextStep{
		DataKey:  "name",
		NextStep: "next",
		EscapeOptions: []ChoiceOption{
			{Label: "Cancelar", Value: "cancel", Finish: true},
		},
	}
	transition := step.Process(Input{CallbackData: "cancel"}, Data{})
	if transition.kind != outcomeComplete {
		t.Fatalf("transition.kind = %v, want outcomeComplete", transition.kind)
	}
}

func TestTextStep_Process_EscapeOption_CallsOnEscape(t *testing.T) {
	step := TextStep{
		DataKey:  "name",
		NextStep: "next",
		EscapeOptions: []ChoiceOption{
			{Label: "Cancelar", Value: "cancel", Finish: true},
		},
		OnEscape: func(value string, data Data) Data {
			next := Data{}
			for k, v := range data {
				next[k] = v
			}
			next["cancelled"] = "true"
			return next
		},
	}
	transition := step.Process(Input{CallbackData: "cancel"}, Data{})
	if transition.data["cancelled"] != "true" {
		t.Errorf("OnEscape should have set cancelled=true, got %+v", transition.data)
	}
}

func TestTextStep_Process_NonEscapeCallback_FallsThroughToTextValidation(t *testing.T) {
	step := TextStep{
		DataKey:  "name",
		NextStep: "next",
		Validate: func(text string, _ Data) string {
			if text == "" {
				return "empty not allowed"
			}
			return ""
		},
		EscapeOptions: []ChoiceOption{
			{Label: "Cancelar", Value: "cancel", Finish: true},
		},
	}
	transition := step.Process(Input{CallbackData: "some_other_button"}, Data{})
	if transition.kind != outcomeRetry {
		t.Fatalf("transition.kind = %v, want outcomeRetry (empty text should fail Validate)", transition.kind)
	}
}

func TestTextStep_Prompt_IncludesEscapeButtons(t *testing.T) {
	step := TextStep{
		PromptText: func(Data) string { return "¿Nombre?" },
		EscapeOptions: []ChoiceOption{
			{Label: "🚫 Cancelar", Value: "cancel", Finish: true},
		},
	}
	prompt := step.Prompt(Data{})
	if len(prompt.Buttons) != 1 || prompt.Buttons[0].Data != "cancel" {
		t.Errorf("prompt.Buttons = %+v, want one button with Data=\"cancel\"", prompt.Buttons)
	}
}

func TestTextStep_Prompt_NoEscapeOptions_NoButtons(t *testing.T) {
	step := TextStep{PromptText: func(Data) string { return "¿Nombre?" }}
	prompt := step.Prompt(Data{})
	if len(prompt.Buttons) != 0 {
		t.Errorf("prompt.Buttons = %+v, want none when EscapeOptions is nil", prompt.Buttons)
	}
}

func TestTextStep_PossibleNextSteps_IncludesEscapeDestinations(t *testing.T) {
	step := TextStep{
		NextStep: "next",
		EscapeOptions: []ChoiceOption{
			{Label: "Atrás", Value: "back", NextStep: "previous"},
			{Label: "Cancelar", Value: "cancel", Finish: true},
		},
	}
	steps := step.PossibleNextSteps()
	want := map[string]bool{"next": true, "previous": true}
	if len(steps) != 2 {
		t.Fatalf("PossibleNextSteps() = %v, want exactly 2 entries (Finish options excluded)", steps)
	}
	for _, s := range steps {
		if !want[s] {
			t.Errorf("unexpected step %q in %v", s, steps)
		}
	}
}
