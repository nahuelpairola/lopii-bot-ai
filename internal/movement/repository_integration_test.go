//go:build integration

package movement

import (
	"testing"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/database"
)

// Run with: go test -tags integration ./internal/movement/
// Requires local Postgres (docker compose up -d) with migrations applied.
func TestInsertAccountsWithOpenings_RollsBackOnFailure(t *testing.T) {
	conn := testConnection(t)
	r := InitRepository(conn)

	userID := uint64(1) // use the test admin user that exists in DB
	before := countAccounts(t, conn, userID)

	items := []AccountOpening{
		{
			Account:  &account.Account{UserID: userID, Name: "TestBank_Rollback_1", Currency: currency.ARS},
			Movement: Movement{UserID: userID, SubcategoryID: 1, Type: Transfer, Amount: decimal.NewFromInt(100), Currency: currency.ARS},
		},
		{
			// SubcategoryID 0 violates the movements.subcategory_id FK → forces
			// a mid-transaction failure on the SECOND item's movement, rolling
			// back the FIRST account that was just created.
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

// testConnection connects to the local Docker Postgres instance used by integration tests.
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

// countAccounts returns the number of non-deleted accounts for the given userID.
func countAccounts(t *testing.T, conn *database.Connection, userID uint64) int64 {
	var count int64
	if err := conn.DB.Model(&account.Account{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		t.Fatalf("failed to count accounts: %v", err)
	}
	return count
}
