package movement

import "testing"

func TestLastTransactionStore_SetGet(t *testing.T) {
	store := NewLastTransactionStore()
	ms := []Movement{{UserID: 1}}

	store.Set(1, ms)

	got, ok := store.Get(1)
	if !ok {
		t.Fatal("expected a hit after Set")
	}
	if len(got) != 1 {
		t.Errorf("got %d movements, want 1", len(got))
	}
}

func TestLastTransactionStore_GetMiss(t *testing.T) {
	store := NewLastTransactionStore()
	if _, ok := store.Get(999); ok {
		t.Error("expected a miss for a user with nothing stored")
	}
}

func TestLastTransactionStore_Clear(t *testing.T) {
	store := NewLastTransactionStore()
	store.Set(1, []Movement{{UserID: 1}})
	store.Clear(1)

	if _, ok := store.Get(1); ok {
		t.Error("expected a miss after Clear")
	}
}
