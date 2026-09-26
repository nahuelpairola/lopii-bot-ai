//go:build integration

package movement

import (
	"testing"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/database"
)

func TestInsertAccountsWithOpenings_RollsBackOnFailure(t *testing.T) {
	conn := testConnection(t)
	r := InitRepository(conn)

	userID := uint64(1)
	before := countAccounts(t, conn, userID)

	items := []AccountOpening{
		{
			Account:  &account.Account{UserID: userID, Name: "TestBank_Rollback_1", Currency: currency.ARS},
			Movement: Movement{UserID: userID, SubcategoryID: 1, Type: Transfer, Amount: decimal.NewFromInt(100), Currency: currency.ARS},
		},
		{
			Account:  &account.Account{UserID: userID, Name: "TestBank_Rollback_2", Currency: currency.ARS},
			Movement: Movement{UserID: userID, SubcategoryID: 0, Type: Transfer, Amount: decimal.NewFromInt(50), Currency: currency.ARS},
		},
	}

	if err := r.InsertAccountsWithOpenings(items); err == nil {
		t.Fatal("expected an error from the FK violation on the second opening movement")
	}
	if after := countAccounts(t, conn, userID); after != before {
		t.Fatalf("rollback failed: accounts went from %d to %d, want unchanged", before, after)
	}
}

func TestInsertBatch_MovementWithoutAccount_IsRejectedByDatabase(t *testing.T) {
	conn := testConnection(t)
	r := InitRepository(conn)

	ms := []Movement{{UserID: 1, SubcategoryID: 1, Type: Expense, Amount: decimal.NewFromInt(-100), Currency: currency.ARS}}
	err := r.InsertBatch(ms)
	if ms[0].ID != 0 {
		t.Cleanup(func() { conn.DB.Unscoped().Delete(&Movement{}, ms[0].ID) })
	}
	if err == nil {
		t.Fatal("a movement with no account_id was stored; the database must reject it")
	}
}

func testConnection(t *testing.T) *database.Connection {
	creds := database.Creds{
		Host:     "localhost",
		Name:     "lopiibot",
		Port:     5432,
		User:     "lopiibot",
		Password: "lopiibot",
	}
	conn, err := database.Initialize(creds, false)
	if err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}
	return conn
}

func countAccounts(t *testing.T, conn *database.Connection, userID uint64) int64 {
	var count int64
	if err := conn.DB.Model(&account.Account{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		t.Fatalf("failed to count accounts: %v", err)
	}
	return count
}
