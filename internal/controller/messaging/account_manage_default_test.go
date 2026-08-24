package messaging

import (
	"context"
	"testing"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
)

func defaultData() conversation.Data {
	return conversation.Data{
		conversation.UserIDKey: uint64(1),
		"operation":            "default",
		"account_id":           "7",
		"account_name":         "Galicia",
		"account_currency":     "ARS",
	}
}

// prevAccount builds the old default of ARS with the given id/name.
func prevAccount(id uint64, name string) *account.Account {
	a := acct(id, currency.ARS, true)
	a.Name = name
	return &a
}

func newDefaultController(prev *account.Account, prevBalance string) (*controller, *fakeAccountRepoFull, *fakeMovementRepoFull, *fakeStoreForController) {
	accRepo := &fakeAccountRepoFull{byCurrency: map[currency.Currency]*account.Account{}}
	movRepo := &fakeMovementRepoFull{balances: map[uint64]string{}}
	if prev != nil {
		accRepo.byCurrency[currency.ARS] = prev
		movRepo.balances[uint64(prev.ID)] = prevBalance
	}
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(flow.NewAccountMoveOfferFlow())
	c := &controller{accounts: accRepo, movements: movRepo, engine: engine, metrics: &fakeMetricRepo{}}
	return c, accRepo, movRepo, store
}

// (a) + (g) prev is another account with balance ≠ 0 → Unset+Set applied,
// default_set resolved, move offer started. The offer starting at all proves
// prev was captured BEFORE UnsetDefault (the fake clears byCurrency on unset).
func TestFinishAccountDefault_OffersMove(t *testing.T) {
	c, accRepo, _, store := newDefaultController(prevAccount(3, "Wallet"), "12000")
	c.finishAccountDefault(context.Background(), &messenger.FakeChat{}, defaultData())

	if len(accRepo.unsetCalls) != 1 || accRepo.unsetCalls[0] != currency.ARS {
		t.Errorf("unsetCalls = %v, want [ARS]", accRepo.unsetCalls)
	}
	if accRepo.setDefaultID != 7 {
		t.Errorf("setDefaultID = %d, want 7", accRepo.setDefaultID)
	}
	if store.flowName != flow.AccountMoveOfferFlowName {
		t.Fatalf("flowName = %q, want %q (offer must start)", store.flowName, flow.AccountMoveOfferFlowName)
	}
	if store.data["move_from_id"] != "3" || store.data["move_to_id"] != "7" {
		t.Errorf("seed from/to = %v/%v, want 3/7", store.data["move_from_id"], store.data["move_to_id"])
	}
}

// (b) no previous default → apply, no offer.
func TestFinishAccountDefault_NoPrev_NoOffer(t *testing.T) {
	c, accRepo, _, store := newDefaultController(nil, "")
	c.finishAccountDefault(context.Background(), &messenger.FakeChat{}, defaultData())

	if accRepo.setDefaultID != 7 {
		t.Errorf("setDefaultID = %d, want 7", accRepo.setDefaultID)
	}
	if store.found {
		t.Error("no previous default → no move offer")
	}
}

// (c) previous default is the same account → apply (idempotent), no offer.
func TestFinishAccountDefault_PrevSameAccount_NoOffer(t *testing.T) {
	c, _, _, store := newDefaultController(prevAccount(7, "Galicia"), "12000")
	c.finishAccountDefault(context.Background(), &messenger.FakeChat{}, defaultData())

	if store.found {
		t.Error("prev == new account → no move offer")
	}
}

// (d) previous default distinct but with balance 0 → apply, no offer.
func TestFinishAccountDefault_PrevZeroBalance_NoOffer(t *testing.T) {
	c, _, _, store := newDefaultController(prevAccount(3, "Wallet"), "0")
	c.finishAccountDefault(context.Background(), &messenger.FakeChat{}, defaultData())

	if store.found {
		t.Error("prev balance 0 → no move offer")
	}
}

// (e) move choice → ReassignAccount called exactly once with from/to.
func TestFinishAccountMoveOffer_Move(t *testing.T) {
	movRepo := &fakeMovementRepoFull{}
	c := &controller{movements: movRepo}
	data := conversation.Data{
		conversation.UserIDKey: uint64(1),
		"move_choice":          "move",
		"move_from_id":         "3",
		"move_from_name":       "Wallet",
		"move_to_id":           "7",
		"move_to_name":         "Galicia",
	}
	c.finishAccountMoveOffer(context.Background(), &messenger.FakeChat{}, data)

	if movRepo.reassignCalls != 1 || movRepo.reassignFrom != 3 || movRepo.reassignTo != 7 {
		t.Errorf("reassign calls=%d from=%d to=%d, want 1/3/7", movRepo.reassignCalls, movRepo.reassignFrom, movRepo.reassignTo)
	}
}

// (f) keep choice → ReassignAccount never called.
func TestFinishAccountMoveOffer_Keep(t *testing.T) {
	movRepo := &fakeMovementRepoFull{}
	c := &controller{movements: movRepo}
	data := conversation.Data{
		conversation.UserIDKey: uint64(1),
		"move_choice":          "keep",
		"move_from_id":         "3",
		"move_to_id":           "7",
	}
	c.finishAccountMoveOffer(context.Background(), &messenger.FakeChat{}, data)

	if movRepo.reassignCalls != 0 {
		t.Errorf("reassign calls = %d, want 0 on keep", movRepo.reassignCalls)
	}
}
