//go:build integration

package movement

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/database"
	"lopiibot.com/internal/subcategory"
)

// Run with: go test -tags integration ./internal/movement/
// Requires local Postgres (docker compose up -d) with migrations applied.
//
// Tres movimientos que matchean por VÍAS DISTINTAS, y uno que no matchea por
// ninguna. Es la única forma de probar que el OR de tres patas está entero: un
// fake devuelve lo que le pongas y nunca ejerce el SQL.
//
// Los acentos y las mayúsculas están mezclados a propósito. El término de
// búsqueda va sin tilde y en minúscula, que es como lo escribe un usuario:
// ninguna de las tres filas matchearía con el `=` exacto y el `ILIKE` de antes.
func TestMovementQuery_Search_MatchesAllThreeLegs(t *testing.T) {
	conn := testConnection(t)
	r := InitRepository(conn)
	accRepo := account.NewRepository(conn)
	userID := uint64(1) // test admin user que ya existe en la DB

	acc := &account.Account{UserID: userID, Name: "Search_" + uuid.NewString()[:8], Currency: currency.ARS}
	if err := accRepo.Insert(acc); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accID := uint64(acc.ID)

	// Taxonomía propia del test: los nombres llevan tilde, el término no.
	byCategory := ownSubcategory(t, conn, userID, "Panadería", "Facturas")
	bySubcategory := ownSubcategory(t, conn, userID, "Comida", "Panadería")
	byDescription := ownSubcategory(t, conn, userID, "Comida", "Super")
	noMatch := ownSubcategory(t, conn, userID, "Transporte", "Nafta")

	// Fecha fija propia, para no pisar movimientos reales del usuario de prueba.
	day := time.Date(2032, 5, 10, 0, 0, 0, 0, time.UTC)
	algo, delaEsquina, nafta := "algo", "pan de la PANADERÍA de la esquina", "nafta"
	if err := r.InsertBatch([]Movement{
		{UserID: userID, AccountID: &accID, SubcategoryID: byCategory, Date: day, Type: Expense, Amount: decimal.NewFromInt(-100), Currency: currency.ARS, Description: &algo},
		{UserID: userID, AccountID: &accID, SubcategoryID: bySubcategory, Date: day, Type: Expense, Amount: decimal.NewFromInt(-200), Currency: currency.ARS, Description: &algo},
		{UserID: userID, AccountID: &accID, SubcategoryID: byDescription, Date: day, Type: Expense, Amount: decimal.NewFromInt(-300), Currency: currency.ARS, Description: &delaEsquina},
		{UserID: userID, AccountID: &accID, SubcategoryID: noMatch, Date: day, Type: Expense, Amount: decimal.NewFromInt(-400), Currency: currency.ARS, Description: &nafta},
	}); err != nil {
		t.Fatalf("insert movements: %v", err)
	}

	term := "panaderia" // sin tilde y en minúscula
	q := MovementQuery{
		UserID:    userID,
		From:      day.AddDate(0, 0, -1),
		To:        day.AddDate(0, 0, 1),
		Currency:  currency.ARS,
		AccountID: &accID,
		Search:    &term,
	}

	rows, err := r.ListForUser(q, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("search %q tiene que traer las 3 patas del OR y NO el control negativo; trajo %d", term, len(rows))
	}
}

// ownSubcategory crea una subcategoría del usuario y devuelve su id. Son del
// usuario y no globales para no ensuciar la taxonomía compartida.
func ownSubcategory(t *testing.T, conn *database.Connection, userID uint64, category, sub string) uint64 {
	t.Helper()
	s := subcategory.Subcategory{UserID: &userID, Category: category, Subcategory: sub + "_" + uuid.NewString()[:6]}
	if err := conn.DB.Create(&s).Error; err != nil {
		t.Fatalf("insert subcategory %q|%q: %v", category, sub, err)
	}
	return uint64(s.ID)
}
