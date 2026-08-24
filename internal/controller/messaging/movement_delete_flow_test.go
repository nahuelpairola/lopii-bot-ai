package messaging

import (
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
)

func TestMovementDeleteFlow_SingleCandidate_SkipsPicker(t *testing.T) {
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(flow.NewMovementDeleteFlow())

	seed := conversation.Data{
		"resolved_index": "0",
		"candidate_groups": flow.EncodeCandidateGroups(
			[]flow.CandidateGroup{{OldIDs: []string{"1"}, Rows: []movement.MovementRow{{}}}},
		),
	}

	prompt, err := engine.StartWithData(1, flow.MovementDeleteFlowName, seed)
	if err != nil {
		t.Fatalf("StartWithData: %v", err)
	}
	if store.stepName != flow.StepConfirmDelete {
		t.Errorf("landed on step %q, want %q (should skip the picker)", store.stepName, flow.StepConfirmDelete)
	}
	_ = prompt
}

func TestMovementDeleteFlow_Ambiguous_ShowsPicker(t *testing.T) {
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(flow.NewMovementDeleteFlow())

	seed := conversation.Data{
		"candidate_labels": conversation.EncodeStringSlice([]string{"🔴 3000 ARS · Café", "🔴 3200 ARS · Café"}),
		"candidate_groups": flow.EncodeCandidateGroups(
			[]flow.CandidateGroup{
				{OldIDs: []string{"1"}, Rows: []movement.MovementRow{{}}},
				{OldIDs: []string{"2"}, Rows: []movement.MovementRow{{}}},
			},
		),
	}

	_, err := engine.StartWithData(1, flow.MovementDeleteFlowName, seed)
	if err != nil {
		t.Fatalf("StartWithData: %v", err)
	}
	if store.stepName != flow.StepPickDeleteCandidate {
		t.Errorf("landed on step %q, want %q (should show the picker)", store.stepName, flow.StepPickDeleteCandidate)
	}
}

func TestFinishMovementDeleteFlow_Confirmed_Deletes(t *testing.T) {
	movRepo := &fakeMovementRepoFull{}
	c := &controller{movements: movRepo}

	data := conversation.Data{
		conversation.UserIDKey: uint64(1),
		"confirmed":            "true",
		"resolved_index":       "0",
		"candidate_groups": flow.EncodeCandidateGroups(
			[]flow.CandidateGroup{{OldIDs: []string{"42"}, Rows: []movement.MovementRow{{}}}},
		),
	}

	flow.FinishMovementDelete(nil, c, &messenger.FakeChat{}, data)

	if len(movRepo.deletedIDs) != 1 || movRepo.deletedIDs[0] != 42 {
		t.Errorf("deletedIDs = %v, want [42]", movRepo.deletedIDs)
	}
}

func TestFinishMovementDeleteFlow_Cancelled_NoDelete(t *testing.T) {
	movRepo := &fakeMovementRepoFull{}
	c := &controller{movements: movRepo}

	data := conversation.Data{
		conversation.UserIDKey: uint64(1),
		"confirmed":            "false",
	}

	flow.FinishMovementDelete(nil, c, &messenger.FakeChat{}, data)

	if len(movRepo.deletedIDs) != 0 {
		t.Error("cancelling should never delete anything")
	}
}
