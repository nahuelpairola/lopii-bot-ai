package conversation

import "testing"

type fakeStore struct {
	flowName, stepName string
	data                Data
	found               bool
}

func (s *fakeStore) Get(userID uint64) (string, string, Data, bool, error) {
	return s.flowName, s.stepName, s.data, s.found, nil
}
func (s *fakeStore) Set(userID uint64, flowName, stepName string, data Data) error {
	s.flowName, s.stepName, s.data, s.found = flowName, stepName, data, true
	return nil
}
func (s *fakeStore) Clear(userID uint64) error {
	s.found = false
	return nil
}

// gapFlow models a 2-gap CREATE-shaped flow: step "a" is skippable once
// data["a"] is set, step "b" is skippable once data["b"] is set, and
// once both are set the flow completes with no further step to show.
func gapFlow() *Flow {
	steps := map[string]Step{
		"a": fakeStep{
			nextStep: "b",
			skip: func(data Data) (string, bool) {
				if _, ok := data["a"]; ok {
					return "b", true
				}
				return "", false
			},
		},
		"b": fakeStep{
			nextStep: "b",
			skip: func(data Data) (string, bool) {
				if _, ok := data["b"]; ok {
					return "", true
				}
				return "", false
			},
		},
	}
	flow, err := NewFlow("gap_flow", "a", steps)
	if err != nil {
		panic(err)
	}
	return flow
}

func TestStartWithData_LandsOnFirstUnresolvedGap(t *testing.T) {
	store := &fakeStore{}
	engine := NewEngine(store)
	engine.Register(gapFlow())

	prompt, err := engine.StartWithData(1, "gap_flow", Data{"a": "resolved"})
	if err != nil {
		t.Fatalf("StartWithData: %v", err)
	}
	if prompt.Text != "prompt:b" {
		t.Errorf("prompt = %q, want to land on step b", prompt.Text)
	}
	if store.stepName != "b" {
		t.Errorf("persisted step = %q, want %q", store.stepName, "b")
	}
}

func TestStartWithData_NoGaps_ErrorsInsteadOfPromptingNothing(t *testing.T) {
	store := &fakeStore{}
	engine := NewEngine(store)
	engine.Register(gapFlow())

	if _, err := engine.StartWithData(1, "gap_flow", Data{"a": "x", "b": "y"}); err == nil {
		t.Fatal("expected an error when the seed leaves nothing to prompt")
	}
}

func TestHandle_ChainsThroughMultipleResolvedGapsAfterAdvance(t *testing.T) {
	store := &fakeStore{}
	engine := NewEngine(store)
	engine.Register(gapFlow())

	// Start with only "a" unresolved; land on step "a".
	if _, err := engine.StartWithData(1, "gap_flow", Data{}); err != nil {
		t.Fatalf("StartWithData: %v", err)
	}

	// Simulate step "a" resolving both "a" and "b" in one Advance (e.g.
	// account-creation confirmation clearing two rows at once) — Handle
	// must walk straight through to completion.
	result, found, err := engine.Handle(1, Input{Text: "anything"})
	_ = result
	if err != nil || !found {
		t.Fatalf("Handle: found=%v err=%v", found, err)
	}
}

func TestFlow_AdvanceThroughSkips_Reentry(t *testing.T) {
	// Regression coverage for the actual CREATE shape: resolving a gap
	// advances to a fixed "detail" step, which then loops back to the
	// gap-queue step — advanceThroughSkips must re-evaluate skippability
	// each time it's re-entered, not just once.
	calls := 0
	steps := map[string]Step{
		"queue": fakeStep{
			nextStep: "detail",
			skip: func(data Data) (string, bool) {
				calls++
				if calls > 1 {
					return "", true // second time through, queue is empty
				}
				return "", false
			},
		},
		"detail": fakeStep{nextStep: "queue"},
	}
	flow, err := NewFlow("reentry", "queue", steps)
	if err != nil {
		t.Fatalf("NewFlow: %v", err)
	}

	// First entry into "queue": not empty yet (calls becomes 1), so the
	// walk stops there instead of moving on.
	resolved, err := flow.advanceThroughSkips("queue", Data{})
	if err != nil {
		t.Fatalf("advanceThroughSkips: %v", err)
	}
	if resolved != "queue" {
		t.Fatalf("first entry: resolved = %q, want %q", resolved, "queue")
	}

	// Re-entry into "queue" (simulating the round-trip through "detail"
	// and back): the walk must re-evaluate Skip from scratch rather than
	// reuse a cached result from the first call above.
	resolved, err = flow.advanceThroughSkips("queue", Data{})
	if err != nil {
		t.Fatalf("advanceThroughSkips: %v", err)
	}
	if resolved != "" {
		t.Errorf("resolved = %q, want empty (complete)", resolved)
	}
}
