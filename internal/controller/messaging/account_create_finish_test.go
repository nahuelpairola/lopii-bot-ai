package messaging

import (
	"context"
	"errors"
	"testing"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/subcategory"
)

func accountCreateTestData(name, cur, balance string) conversation.Data {
	return conversation.Data{
		conversation.UserIDKey: uint64(1),
		"account_name":         name,
		"account_currency":     cur,
		"account_balance":      balance,
	}
}

func TestFinishAccountCreateFlow_Cancelled_NoDBWrite(t *testing.T) {
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{}
	c := &controller{accounts: accRepo, movements: movRepo, subcategories: &fakeSubcategoryRepoFull{}}

	data := accountCreateTestData("Jubilación", "ARS", "0")
	data["cancelled"] = "true"

	c.finishAccountCreateFlow(context.Background(), &messenger.FakeChat{}, data)

	if len(accRepo.inserted) != 0 {
		t.Error("a cancelled flow should never insert an account")
	}
	if len(movRepo.inserted) != 0 {
		t.Error("a cancelled flow should never insert a movement")
	}
}

func TestFinishAccountCreateFlow_Success_InsertsAccountAndOpeningMovement(t *testing.T) {
	sub := &subcategory.Subcategory{}
	sub.ID = 7
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{}
	c := &controller{
		accounts:      accRepo,
		movements:     movRepo,
		subcategories: &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{"Sistema|Saldo inicial": sub}},
	}

	c.finishAccountCreateFlow(context.Background(), &messenger.FakeChat{}, accountCreateTestData("Jubilación", "ARS", "50000"))

	if len(accRepo.inserted) != 1 {
		t.Fatalf("expected 1 account inserted, got %d", len(accRepo.inserted))
	}
	got := accRepo.inserted[0]
	if got.Name != "Jubilación" || got.Currency != currency.ARS || got.IsDefault {
		t.Errorf("inserted account = %+v, want Name=Jubilación Currency=ARS IsDefault=false", got)
	}

	if len(movRepo.inserted) != 1 {
		t.Fatalf("expected 1 opening movement inserted, got %d", len(movRepo.inserted))
	}
	m := movRepo.inserted[0]
	if m.SubcategoryID != 7 {
		t.Errorf("movement.SubcategoryID = %d, want 7", m.SubcategoryID)
	}
	if m.AccountID == nil || *m.AccountID != uint64(got.ID) {
		t.Errorf("movement.AccountID = %v, want %d", m.AccountID, got.ID)
	}
	if m.Amount.String() != "50000" {
		t.Errorf("movement.Amount = %s, want 50000", m.Amount.String())
	}
}

func TestFinishAccountCreateFlow_DuplicateName_NoMovementInserted(t *testing.T) {
	accRepo := &fakeAccountRepoFull{insertErr: account.ErrAccountAlreadyExists}
	movRepo := &fakeMovementRepoFull{}
	c := &controller{accounts: accRepo, movements: movRepo, subcategories: &fakeSubcategoryRepoFull{}}

	c.finishAccountCreateFlow(context.Background(), &messenger.FakeChat{}, accountCreateTestData("Wallet", "ARS", "0"))

	if len(movRepo.inserted) != 0 {
		t.Error("a duplicate-name failure should never insert a movement")
	}
}

func TestFinishAccountCreateFlow_MovementInsertFails_Propagates(t *testing.T) {
	sub := &subcategory.Subcategory{}
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{insertErr: errors.New("db down")}
	c := &controller{
		accounts:      accRepo,
		movements:     movRepo,
		subcategories: &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{"Sistema|Saldo inicial": sub}},
	}

	c.finishAccountCreateFlow(context.Background(), &messenger.FakeChat{}, accountCreateTestData("Jubilación", "ARS", "0"))

	if len(accRepo.inserted) != 1 {
		t.Error("the account should still have been created before the movement insert failed")
	}
}
