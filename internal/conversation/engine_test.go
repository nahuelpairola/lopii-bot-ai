package conversation

import (
	"testing"
	"time"
)

type fakeStore struct {
	flowName, stepName string
	data               Data
	updatedAt          time.Time
	found              bool
}

func (s *fakeStore) Get(userID uint64) (string, string, Data, time.Time, bool, error) {
	return s.flowName, s.stepName, s.data, s.updatedAt, s.found, nil
}
func (s *fakeStore) Set(userID uint64, flowName, stepName string, data Data) error {
	s.flowName, s.stepName, s.data, s.found = flowName, stepName, data, true
	s.updatedAt = time.Now()
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
	engine := NewEngine(store, func(string) string { return "algo" })
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
	engine := NewEngine(store, func(string) string { return "algo" })
	engine.Register(gapFlow())

	if _, err := engine.StartWithData(1, "gap_flow", Data{"a": "x", "b": "y"}); err == nil {
		t.Fatal("expected an error when the seed leaves nothing to prompt")
	}
}

func TestHandle_ChainsThroughMultipleResolvedGapsAfterAdvance(t *testing.T) {
	// Dedicated flow (not gapFlow): step "a"'s process actually writes
	// both data["a"] and data["b"] before advancing to "b" — simulating,
	// e.g., an account-creation confirmation that clears two gap rows in
	// one Advance. This proves Handle re-evaluates skippability on "b"
	// after the advance (data["b"] is now present, so "b" is skippable
	// and the flow reaches completion) rather than just showing "b"'s
	// Prompt, which is what would happen without the post-advance
	// advanceThroughSkips walk this task adds.
	steps := map[string]Step{
		"a": fakeStep{
			nextStep: "b",
			skip: func(data Data) (string, bool) {
				if _, ok := data["a"]; ok {
					return "b", true
				}
				return "", false
			},
			process: func(input Input, data Data) Transition {
				next := Data{}
				for k, v := range data {
					next[k] = v
				}
				next["a"] = "x"
				next["b"] = "y"
				return Advance("b", next)
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
	flow, err := NewFlow("chain_flow", "a", steps)
	if err != nil {
		t.Fatalf("NewFlow: %v", err)
	}

	store := &fakeStore{}
	engine := NewEngine(store, func(string) string { return "algo" })
	engine.Register(flow)

	// Start with nothing resolved; land on step "a".
	if _, err := engine.StartWithData(1, "chain_flow", Data{}); err != nil {
		t.Fatalf("StartWithData: %v", err)
	}

	result, found, err := engine.Handle(1, Input{Text: "anything"})
	if err != nil || !found {
		t.Fatalf("Handle: found=%v err=%v", found, err)
	}
	if !result.Finished {
		t.Fatalf("expected Handle to walk through both resolved gaps and finish; got Finished=false (result=%+v)", result)
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

func TestHandle_IdleThreshold_TriggersResumeGate(t *testing.T) {
	store := &fakeStore{flowName: "gap_flow", stepName: "a", data: Data{}, updatedAt: time.Now().Add(-11 * time.Minute), found: true}
	engine := NewEngine(store, func(string) string { return "algo" })
	engine.Register(gapFlow())

	result, found, err := engine.Handle(1, Input{Text: "anything"})
	if err != nil || !found {
		t.Fatalf("Handle: found=%v err=%v", found, err)
	}
	if len(result.Prompt.Buttons) != 2 {
		t.Fatalf("expected the resume-gate's 2 buttons, got %d", len(result.Prompt.Buttons))
	}
}

func TestHandle_SecondConsecutiveRetry_EscalatesToResumeGate(t *testing.T) {
	steps := map[string]Step{
		"a": fakeStep{nextStep: "a", process: func(input Input, data Data) Transition { return Retry("nope") }},
	}
	flow, err := NewFlow("retry_flow", "a", steps)
	if err != nil {
		t.Fatalf("NewFlow: %v", err)
	}

	store := &fakeStore{}
	engine := NewEngine(store, func(string) string { return "algo" })
	engine.Register(flow)
	if _, err := engine.StartWithData(1, "retry_flow", Data{}); err != nil {
		t.Fatalf("StartWithData: %v", err)
	}

	first, _, err := engine.Handle(1, Input{Text: "bad"})
	if err != nil {
		t.Fatalf("first Handle: %v", err)
	}
	if len(first.Prompt.Buttons) != 0 {
		t.Fatalf("first mismatch should be a plain retry, not the gate; got buttons %+v", first.Prompt.Buttons)
	}

	second, _, err := engine.Handle(1, Input{Text: "bad again"})
	if err != nil {
		t.Fatalf("second Handle: %v", err)
	}
	if len(second.Prompt.Buttons) != 2 {
		t.Fatalf("second consecutive mismatch should escalate to the gate; got buttons %+v", second.Prompt.Buttons)
	}
}

func TestHandle_ResumeContinue_ResendsCurrentPrompt(t *testing.T) {
	store := &fakeStore{}
	engine := NewEngine(store, func(string) string { return "algo" })
	engine.Register(gapFlow())
	if _, err := engine.StartWithData(1, "gap_flow", Data{}); err != nil {
		t.Fatalf("StartWithData: %v", err)
	}

	result, found, err := engine.Handle(1, Input{CallbackData: "_resume_continue"})
	if err != nil || !found {
		t.Fatalf("Handle: found=%v err=%v", found, err)
	}
	if result.Finished {
		t.Fatal("resume_continue must not finish the flow")
	}
}

func TestHandle_ResumeContinue_UnregisteredFlow_ReturnsErrorNotPanic(t *testing.T) {
	store := &fakeStore{
		flowName:  "some_removed_flow",
		stepName:  "some_step",
		data:      Data{},
		updatedAt: time.Now(),
		found:     true,
	}
	engine := NewEngine(store, func(string) string { return "algo" })

	_, found, err := engine.Handle(1, Input{CallbackData: "_resume_continue"})
	if err == nil {
		t.Fatal("expected error for unregistered flow, got nil")
	}
	if !found {
		t.Fatal("found should be true: state row exists, it's just an unregistered flow")
	}
}

func TestHandle_ResumeCancel_ClearsStateAndFinishesWithMarker(t *testing.T) {
	store := &fakeStore{}
	engine := NewEngine(store, func(string) string { return "algo" })
	engine.Register(gapFlow())
	if _, err := engine.StartWithData(1, "gap_flow", Data{}); err != nil {
		t.Fatalf("StartWithData: %v", err)
	}

	result, found, err := engine.Handle(1, Input{CallbackData: "_resume_cancel"})
	if err != nil || !found {
		t.Fatalf("Handle: found=%v err=%v", found, err)
	}
	if !result.Finished || result.Data[ResumeCancelledKey] != "true" {
		t.Fatalf("expected Finished with _resume_cancelled marker, got %+v", result)
	}
	if store.found {
		t.Error("expected state.Clear to have been called")
	}
}
