//go:build integration

package movement

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
)

func TestReassignAccount_VoidsInternalTransfers_RepointsExternals(t *testing.T) {
	conn := testConnection(t)
	r := InitRepository(conn)
	accRepo := account.NewRepository(conn)
	userID := uint64(1)

	accA := &account.Account{UserID: userID, Name: "Reassign_A_" + uuid.NewString()[:8], Currency: currency.ARS}
	accB := &account.Account{UserID: userID, Name: "Reassign_B_" + uuid.NewString()[:8], Currency: currency.ARS}
	accC := &account.Account{UserID: userID, Name: "Reassign_C_" + uuid.NewString()[:8], Currency: currency.ARS}
	for _, a := range []*account.Account{accA, accB, accC} {
		if err := accRepo.Insert(a); err != nil {
			t.Fatalf("insert account: %v", err)
		}
	}
	idA, idB, idC := uint64(accA.ID), uint64(accB.ID), uint64(accC.ID)

	internalTx := uuid.New()
	externalTx := uuid.New()
	mustInsert := func(ms ...Movement) {
		if err := r.InsertBatch(ms); err != nil {
			t.Fatalf("insert movements: %v", err)
		}
	}
	mustInsert(Movement{UserID: userID, AccountID: &idA, SubcategoryID: 1, Type: Expense, Amount: decimal.NewFromInt(-1000), Currency: currency.ARS})
	mustInsert(
		Movement{UserID: userID, AccountID: &idA, SubcategoryID: 1, Type: Transfer, Amount: decimal.NewFromInt(-500), Currency: currency.ARS, TransactionID: &internalTx},
		Movement{UserID: userID, AccountID: &idB, SubcategoryID: 1, Type: Transfer, Amount: decimal.NewFromInt(500), Currency: currency.ARS, TransactionID: &internalTx},
	)
	mustInsert(
		Movement{UserID: userID, AccountID: &idC, SubcategoryID: 1, Type: Transfer, Amount: decimal.NewFromInt(-300), Currency: currency.ARS, TransactionID: &externalTx},
		Movement{UserID: userID, AccountID: &idA, SubcategoryID: 1, Type: Transfer, Amount: decimal.NewFromInt(300), Currency: currency.ARS, TransactionID: &externalTx},
	)

	sumA, _ := r.SumAmountForAccount(idA)
	sumB, _ := r.SumAmountForAccount(idB)
	combined := sumA.Add(sumB)

	if err := r.ReassignAccount(idA, idB); err != nil {
		t.Fatalf("ReassignAccount: %v", err)
	}

	newA, _ := r.SumAmountForAccount(idA)
	if !newA.IsZero() {
		t.Fatalf("saldo de A tras reassign = %s, want 0", newA)
	}
	newB, _ := r.SumAmountForAccount(idB)
	if !newB.Equal(combined) {
		t.Fatalf("saldo de B tras reassign = %s, want %s (bal A+B)", newB, combined)
	}
	var internalLive int64
	conn.DB.Model(&Movement{}).Where("transaction_id = ?", internalTx).Count(&internalLive)
	if internalLive != 0 {
		t.Fatalf("la transferencia interna quedó con %d patas vivas, want 0", internalLive)
	}
	var externalLive int64
	conn.DB.Model(&Movement{}).Where("transaction_id = ?", externalTx).Count(&externalLive)
	if externalLive != 2 {
		t.Fatalf("la transferencia externa quedó con %d patas vivas, want 2", externalLive)
	}
}
