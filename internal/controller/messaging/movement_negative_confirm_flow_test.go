package messaging

import (
	"context"
	"testing"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

func TestNegativeConfirm_RegisterIgualInserts(t *testing.T) {
	subs := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Sistema|Transferencia": newSubForTest(8, "Sistema", "Transferencia"),
		"Inversiones|FCI":       newSubForTest(7, "Inversiones", "FCI"),
	}}
	accts := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true), acct(2, currency.ARS, false)}}
	movs := &fakeMovementRepoFull{balances: map[uint64]string{1: "200000", 2: "0"}}
	c := &controller{subcategories: subs, accounts: accts, movements: movs}

	rows := []movement.MovementRow{
		{Type: "transfer", Amount: "-1000000", Currency: "ARS", Category: "Sistema", Subcategory: "Transferencia", AccountID: "1", Group: "g", Date: "2026-07-07"},
		{Type: "transfer", Amount: "1000000", Currency: "ARS", Category: "Sistema", Subcategory: "Transferencia", AccountID: "2", Group: "g", Date: "2026-07-07"},
	}
	data := conversation.Data{conversation.UserIDKey: uint64(1), "mode": "create", "movements": movement.EncodeMovementRows(rows), "old_movement_ids": conversation.EncodeStringSlice(nil), "_gate_choice": "register"}

	flow.FinishMovementNegativeConfirm(context.Background(), c, nil, 0, data)
	if len(movs.inserted) != 2 {
		t.Fatalf("Registrar igual must insert; got %d movements", len(movs.inserted))
	}
}
