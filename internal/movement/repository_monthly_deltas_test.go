//go:build integration

package movement

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
)

func TestMonthlyDeltasForAccount_SignedAndOrdered(t *testing.T) {
	conn := testConnection(t)
	r := InitRepository(conn)
	accRepo := account.NewRepository(conn)
	userID := uint64(1)

	acc := &account.Account{UserID: userID, Name: "MonthlyDeltas_" + uuid.NewString()[:8], Currency: currency.ARS}
	if err := accRepo.Insert(acc); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID := uint64(acc.ID)

	now := time.Now()
	month1 := time.Date(now.Year(), now.Month(), 1, 12, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	month2 := time.Date(now.Year(), now.Month(), 1, 12, 0, 0, 0, time.UTC)

	mustInsert := func(ms ...Movement) {
		if err := r.InsertBatch(ms); err != nil {
			t.Fatalf("insert movements: %v", err)
		}
	}
	mustInsert(Movement{UserID: userID, AccountID: &accountID, SubcategoryID: 1, Date: month1, Type: Transfer, Amount: decimal.NewFromInt(10000), Currency: currency.ARS})
	mustInsert(Movement{UserID: userID, AccountID: &accountID, SubcategoryID: 1, Date: month2, Type: Transfer, Amount: decimal.NewFromInt(-3000), Currency: currency.ARS})

	deltas, err := r.MonthlyDeltasForAccount(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(deltas) != 2 {
		t.Fatalf("expected 2 months, got %d", len(deltas))
	}
	if !deltas[0].Delta.Equal(decimal.NewFromInt(10000)) {
		t.Fatalf("month 1 delta: expected 10000, got %s", deltas[0].Delta)
	}
	if !deltas[1].Delta.Equal(decimal.NewFromInt(-3000)) {
		t.Fatalf("month 2 delta: expected -3000, got %s", deltas[1].Delta)
	}
	if deltas[0].Month >= deltas[1].Month {
		t.Fatal("expected months in ascending order")
	}
}
