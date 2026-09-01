//go:build integration

package movement

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/database"
	"lopiibot.com/internal/subcategory"
)

func TestSumForUser_ExcludesReservedByDefault_AndOnlyThemWhenAsked(t *testing.T) {
	conn := testConnection(t)
	r := InitRepository(conn)
	accRepo := account.NewRepository(conn)
	userID := uint64(1)

	acc := &account.Account{UserID: userID, Name: "Reserved_" + uuid.NewString()[:8], Currency: currency.ARS}
	if err := accRepo.Insert(acc); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accID := uint64(acc.ID)

	realSub := subcategoryID(t, conn, "Alimentación", "")
	adjustSub := subcategoryID(t, conn, subcategory.CategorySystem, "Ajuste de saldo")
	yieldSub := subcategoryID(t, conn, subcategory.CategorySystem, "Rendimiento inversión")

	day := time.Date(2031, 3, 15, 0, 0, 0, 0, time.UTC)
	if err := r.InsertBatch([]Movement{
		{UserID: userID, AccountID: &accID, SubcategoryID: realSub, Date: day, Type: Expense, Amount: decimal.NewFromInt(-1000), Currency: currency.ARS},
		{UserID: userID, AccountID: &accID, SubcategoryID: adjustSub, Date: day, Type: Expense, Amount: decimal.NewFromInt(-700), Currency: currency.ARS},
		{UserID: userID, AccountID: &accID, SubcategoryID: yieldSub, Date: day, Type: Income, Amount: decimal.NewFromInt(300), Currency: currency.ARS},
	}); err != nil {
		t.Fatalf("insert movements: %v", err)
	}

	expense := constants.Expense
	base := MovementQuery{
		UserID:    userID,
		From:      day.AddDate(0, 0, -1),
		To:        day.AddDate(0, 0, 1),
		Currency:  currency.ARS,
		AccountID: &accID,
	}

	expenseQ := base
	expenseQ.Type = &expense
	got := singleTotal(t, r, expenseQ)
	if !got.Equal(decimal.NewFromInt(1000)) {
		t.Errorf("gastos con el filtro por default = %s, want 1000 (el ajuste de 700 no debe entrar)", got)
	}

	variationQ := base
	variationQ.OnlyReserved = true
	rows, err := r.SumForUser(variationQ, GroupByType)
	if err != nil {
		t.Fatalf("SumForUser(OnlyReserved): %v", err)
	}
	byType := map[string]decimal.Decimal{}
	for _, row := range rows {
		byType[row.Label] = row.Total
	}
	if got := byType[constants.Expense]; !got.Equal(decimal.NewFromInt(700)) {
		t.Errorf("ajuste negativo bajo OnlyReserved = %s, want 700", got)
	}
	if got := byType[constants.Income]; !got.Equal(decimal.NewFromInt(300)) {
		t.Errorf("rendimiento bajo OnlyReserved = %s, want 300", got)
	}
}

func singleTotal(t *testing.T, r *repository, q MovementQuery) decimal.Decimal {
	t.Helper()
	rows, err := r.SumForUser(q, GroupByNone)
	if err != nil {
		t.Fatalf("SumForUser: %v", err)
	}
	if len(rows) == 0 {
		return decimal.Zero
	}
	return rows[0].Total
}

func subcategoryID(t *testing.T, conn *database.Connection, category, sub string) uint64 {
	t.Helper()
	var id uint64
	q := conn.DB.Table("subcategories").Select("id").Where("category = ?", category)
	if sub != "" {
		q = q.Where("subcategory = ?", sub)
	}
	if err := q.Limit(1).Scan(&id).Error; err != nil || id == 0 {
		t.Fatalf("no encontré la subcategoría %q|%q (err=%v). ¿Corriste las migraciones?", category, sub, err)
	}
	return id
}

func TestSumForUser_TransferQuerySeesRealTransfers_ButNotOpeningBalances(t *testing.T) {
	conn := testConnection(t)
	r := InitRepository(conn)
	accRepo := account.NewRepository(conn)
	userID := uint64(1)

	acc := &account.Account{UserID: userID, Name: "Transfer_" + uuid.NewString()[:8], Currency: currency.ARS}
	if err := accRepo.Insert(acc); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accID := uint64(acc.ID)

	transferSub := subcategoryID(t, conn, subcategory.CategorySystem, subcategory.SubTransfer)
	openingSub := subcategoryID(t, conn, subcategory.CategorySystem, subcategory.SubOpeningBalance)

	day := time.Date(2031, 4, 12, 0, 0, 0, 0, time.UTC)
	if err := r.InsertBatch([]Movement{
		{UserID: userID, AccountID: &accID, SubcategoryID: transferSub, Date: day, Type: Transfer, Amount: decimal.NewFromInt(-5000), Currency: currency.ARS},
		{UserID: userID, AccountID: &accID, SubcategoryID: openingSub, Date: day, Type: Transfer, Amount: decimal.NewFromInt(-900), Currency: currency.ARS},
	}); err != nil {
		t.Fatalf("insert movements: %v", err)
	}

	transfer := constants.Transfer
	q := MovementQuery{
		UserID:    userID,
		From:      day.AddDate(0, 0, -1),
		To:        day.AddDate(0, 0, 1),
		Currency:  currency.ARS,
		AccountID: &accID,
		Type:      &transfer,
	}
	if got := singleTotal(t, r, q); !got.Equal(decimal.NewFromInt(5000)) {
		t.Errorf("transferencias = %s, want 5000 (la real entra, el saldo inicial de 900 no)", got)
	}
}
