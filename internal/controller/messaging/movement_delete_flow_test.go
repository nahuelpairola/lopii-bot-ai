package messaging

import (
	"testing"

	"gorm.io/gorm"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
)

type fakeLastTransactionStore struct {
	cleared int
	set     int
	stored  []movement.Movement
}

func (s *fakeLastTransactionStore) Set(userID uint64, movements []movement.Movement) {
	s.set++
	s.stored = movements
}
func (s *fakeLastTransactionStore) Get(userID uint64) ([]movement.Movement, bool) {
	return s.stored, s.stored != nil
}
func (s *fakeLastTransactionStore) Clear(userID uint64) { s.cleared++ }

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
			nil,
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
			nil,
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

func TestFinishMovementDeleteFlow_Confirmed_DeletesAndClears(t *testing.T) {
	movRepo := &fakeMovementRepoFull{}
	lastTx := &fakeLastTransactionStore{}
	c := &controller{movements: movRepo, lastTransactions: lastTx}

	data := conversation.Data{
		conversation.UserIDKey: uint64(1),
		"confirmed":            "true",
		"resolved_index":       "0",
		"candidate_groups": encodeCandidateGroups(
			[]transactionGroup{{Movements: []movement.Movement{{Model: movementModelWithID(t, 42)}}}},
			nil,
		),
	}

	c.finishMovementDeleteFlow(nil, nil, 0, data)

	if lastTx.cleared != 1 {
		t.Errorf("lastTransactions.Clear called for user %d, want 1", lastTx.cleared)
	}
}

func TestFinishMovementDeleteFlow_Cancelled_NoDelete(t *testing.T) {
	movRepo := &fakeMovementRepoFull{}
	lastTx := &fakeLastTransactionStore{}
	c := &controller{movements: movRepo, lastTransactions: lastTx}

	data := conversation.Data{
		conversation.UserIDKey: uint64(1),
		"confirmed":            "false",
	}

	c.finishMovementDeleteFlow(nil, nil, 0, data)

	if lastTx.cleared != 0 {
		t.Error("cancelling should never clear lastTransactions")
	}
}
