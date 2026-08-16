package agent

import (
	"testing"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/subcategory"
)

func TestResolveAndInsertMovements_SimpleSingleMovement_NilTransactionID(t *testing.T) {
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Alimentación|Café": newSubForTest(1, "Alimentación", "Café"),
	}}
	accRepo := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true)}}
	movRepo := &fakeMovementRepoFull{balances: map[uint64]string{1: "1000000"}}

	svc := &fakeServices{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	result := orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{
		{Type: "expense", Amount: "3000", Currency: "ARS", Category: "Alimentación", Subcategory: "Café", PaymentMethod: "cash", Description: "Café", Date: "2026-07-02"},
	}}
	data := buildCreateSeed(result, nil, nil)
	data[conversation.UserIDKey] = uint64(1)

	inserted, err := svc.ResolveAndInsertMovements(data)
	if err != nil {
		t.Fatalf("resolveAndInsertMovements: %v", err)
	}
	if len(movRepo.inserted) != 1 {
		t.Fatalf("got %d inserted, want 1", len(movRepo.inserted))
	}
	if movRepo.inserted[0].TransactionID != nil {
		t.Error("a single movement should have a nil TransactionID")
	}
	if len(inserted) != 1 {
		t.Errorf("resolveAndInsertMovements should return the inserted rows")
	}
}

func TestResolveAndInsertMovements_Compound_SharesTransactionID(t *testing.T) {
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Inversiones|Compra USD": newSubForTest(2, "Inversiones", "Compra USD"),
	}}
	accRepo := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true), acct(7, currency.USD, false)}}
	movRepo := &fakeMovementRepoFull{}
	svc := &fakeServices{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	result := orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{
		{Type: "transfer", Amount: "140000", Currency: "ARS", Category: "Inversiones", Subcategory: "Compra USD", PaymentMethod: "transfer", Description: "Compra USD", Date: "2026-07-02", AccountID: uint64Ptr(1), Group: "g1"},
		{Type: "transfer", Amount: "100", Currency: "USD", Category: "Inversiones", Subcategory: "Compra USD", PaymentMethod: "transfer", Description: "Compra USD", Date: "2026-07-02", AccountID: uint64Ptr(7), Group: "g1"},
	}}
	data := buildCreateSeed(result, nil, nil)
	data[conversation.UserIDKey] = uint64(1)

	if _, err := svc.ResolveAndInsertMovements(data); err != nil {
		t.Fatalf("resolveAndInsertMovements: %v", err)
	}
	if len(movRepo.inserted) != 2 {
		t.Fatalf("got %d inserted, want 2", len(movRepo.inserted))
	}
	if movRepo.inserted[0].TransactionID == nil || movRepo.inserted[1].TransactionID == nil {
		t.Fatal("both rows of a compound transaction should have a TransactionID")
	}
	if *movRepo.inserted[0].TransactionID != *movRepo.inserted[1].TransactionID {
		t.Error("both rows should share the same TransactionID")
	}
}

func TestResolveAndInsertMovements_PopulatesSubcategoryAssociation(t *testing.T) {
	sub := newSubForTest(1, "Alimentación", "Café")
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Alimentación|Café": sub,
	}}
	accRepo := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true)}}
	movRepo := &fakeMovementRepoFull{balances: map[uint64]string{1: "1000000"}}
	svc := &fakeServices{subcategories: subRepo, accounts: accRepo, movements: movRepo}

	result := orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{
		{Type: "expense", Amount: "3000", Currency: "ARS", Category: "Alimentación", Subcategory: "Café", PaymentMethod: "cash", Description: "Café", Date: "2026-07-02"},
	}}
	data := buildCreateSeed(result, nil, nil)
	data[conversation.UserIDKey] = uint64(1)

	if _, err := svc.ResolveAndInsertMovements(data); err != nil {
		t.Fatalf("resolveAndInsertMovements: %v", err)
	}
	if movRepo.inserted[0].Subcategory == nil || movRepo.inserted[0].Subcategory.Category != "Alimentación" {
		t.Errorf("inserted movement's Subcategory = %+v, want Category=Alimentación", movRepo.inserted[0].Subcategory)
	}
}
