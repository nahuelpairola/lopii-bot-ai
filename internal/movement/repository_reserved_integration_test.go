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

// Run with: go test -tags integration ./internal/movement/
// Requires local Postgres (docker compose up -d) with migrations applied.
//
// Esta garantía vive en el SQL de MovementQuery.apply, no en Go: un mock del
// repositorio probaría el mock — que es exactamente por qué el bug existió.
// El filtro de categorías reservadas estaba copiado en cada vista, `overview` y
// `summary` se lo olvidaron, y un ajuste de saldo se rendía como gasto real.
//
// Cubre las dos direcciones: por default las reservadas NO entran en los
// agregados de plata, y con OnlyReserved entran SOLO ellas.
func TestSumForUser_ExcludesReservedByDefault_AndOnlyThemWhenAsked(t *testing.T) {
	conn := testConnection(t)
	r := InitRepository(conn)
	accRepo := account.NewRepository(conn)
	userID := uint64(1) // test admin user que ya existe en la DB

	acc := &account.Account{UserID: userID, Name: "Reserved_" + uuid.NewString()[:8], Currency: currency.ARS}
	if err := accRepo.Insert(acc); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accID := uint64(acc.ID)

	realSub := subcategoryID(t, conn, "Alimentación", "")
	adjustSub := subcategoryID(t, conn, subcategory.CategorySystem, "Ajuste de saldo")
	yieldSub := subcategoryID(t, conn, subcategory.CategorySystem, "Rendimiento inversión")

	// Una fecha fija propia, para que la ventana del query no dependa de "hoy"
	// ni pise movimientos reales del usuario de prueba.
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

	// (1) El default: el gasto real cuenta, el ajuste NO. Si esto se rompe, el
	// KPI "Gastos" del Mini App vuelve a inflarse con revaluaciones de cuenta.
	expenseQ := base
	expenseQ.Type = &expense
	got := singleTotal(t, r, expenseQ)
	if !got.Equal(decimal.NewFromInt(1000)) {
		t.Errorf("gastos con el filtro por default = %s, want 1000 (el ajuste de 700 no debe entrar)", got)
	}

	// (2) OnlyReserved devuelve solo la plomería, con el signo recuperado desde
	// el tipo — SumForUser suma ABS. Type nil de paso descarta los transfer,
	// que es lo que deja afuera saldos iniciales y patas de transferencia.
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

// subcategoryID resolves a seeded global subcategory to its id. sub == "" takes
// any subcategory under that category — the test only needs a row that is NOT
// reserved, not a specific one.
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
