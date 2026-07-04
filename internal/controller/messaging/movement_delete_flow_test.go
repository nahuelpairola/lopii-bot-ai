package messaging

import (
	"testing"

	"gorm.io/gorm"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
)

func movementModelWithID(t *testing.T, id uint) (m gorm.Model) {
	t.Helper()
	m.ID = id
	return m
}

func TestMovementDeleteFlow_SingleCandidate_SkipsPicker(t *testing.T) {
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store)
	engine.Register(NewMovementDeleteFlow())

	seed := conversation.Data{
		"resolved_index": "0",
		"candidate_groups": encodeCandidateGroups(
			[]transactionGroup{{Movements: []movement.Movement{{}}}},
		),
	}

	prompt, err := engine.StartWithData(1, movementDeleteFlowName, seed)
	if err != nil {
		t.Fatalf("StartWithData: %v", err)
	}
	if store.stepName != stepConfirmDelete {
		t.Errorf("landed on step %q, want %q (should skip the picker)", store.stepName, stepConfirmDelete)
	}
	_ = prompt
}

func TestMovementDeleteFlow_Ambiguous_ShowsPicker(t *testing.T) {
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store)
	engine.Register(NewMovementDeleteFlow())

	seed := conversation.Data{
		"candidate_labels": encodeStringSlice([]string{"🔴 3000 ARS · Café", "🔴 3200 ARS · Café"}),
		"candidate_groups": encodeCandidateGroups(
			[]transactionGroup{
				{Movements: []movement.Movement{{}}},
				{Movements: []movement.Movement{{}}},
			},
		),
	}

	_, err := engine.StartWithData(1, movementDeleteFlowName, seed)
	if err != nil {
		t.Fatalf("StartWithData: %v", err)
	}
	if store.stepName != stepPickDeleteCandidate {
		t.Errorf("landed on step %q, want %q (should show the picker)", store.stepName, stepPickDeleteCandidate)
	}
}

func TestFinishMovementDeleteFlow_Confirmed_Deletes(t *testing.T) {
	movRepo := &fakeMovementRepoFull{}
	c := &controller{movements: movRepo}

	data := conversation.Data{
		conversation.UserIDKey: uint64(1),
		"confirmed":            "true",
		"resolved_index":       "0",
		"candidate_groups": encodeCandidateGroups(
			[]transactionGroup{{Movements: []movement.Movement{{Model: movementModelWithID(t, 42)}}}},
		),
	}

	c.finishMovementDeleteFlow(nil, nil, 0, data)

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

	c.finishMovementDeleteFlow(nil, nil, 0, data)

	if len(movRepo.deletedIDs) != 0 {
		t.Error("cancelling should never delete anything")
	}
}
