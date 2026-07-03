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
}

func (s fakeStep) Prompt(data Data) Prompt { return Prompt{Text: "prompt:" + s.nextStep} }
func (s fakeStep) Process(input Input, data Data) Transition {
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
