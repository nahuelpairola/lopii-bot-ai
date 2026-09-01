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

func TestListForAccount_IncludesTransfers(t *testing.T) {
	conn := testConnection(t)
	r := InitRepository(conn)
	accRepo := account.NewRepository(conn)
	userID := uint64(1)

	acc := &account.Account{UserID: userID, Name: "ListForAccount_" + uuid.NewString()[:8], Currency: currency.ARS}
	if err := accRepo.Insert(acc); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID := uint64(acc.ID)

	day := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	if err := r.InsertBatch([]Movement{
		{UserID: userID, AccountID: &accountID, SubcategoryID: 1, Date: day, Type: Transfer, Amount: decimal.NewFromInt(-100000), Currency: currency.ARS},
	}); err != nil {
		t.Fatalf("insert movements: %v", err)
	}

	rows, err := r.ListForAccount(accountID,
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC), 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("esperaba 1 fila (el transfer), got %d — apply lo habría filtrado", len(rows))
	}
	if rows[0].Subcategory == nil {
		t.Fatal("Subcategory tiene que venir precargado aunque esté borrada: la vista la renderiza, y sin ella la fila pierde su nombre")
	}

	empty, err := r.ListForAccount(accountID,
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC), 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("julio no debería traer nada, got %d", len(empty))
	}
}
