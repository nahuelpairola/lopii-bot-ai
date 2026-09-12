//go:build integration

package movement

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/subcategory"
	"lopiibot.com/internal/user"
)

type recurringFixture struct {
	r     *repository
	uid   uint64
	day   time.Time
	arsID uint64
	usdID uint64
}

func newRecurringFixture(t *testing.T) recurringFixture {
	t.Helper()
	conn := testConnection(t)
	u := &user.User{Username: fmt.Sprintf("recurring_%d", time.Now().UnixNano())}
	if err := user.NewRepository(conn).Insert(u); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	uid := u.ID
	t.Cleanup(func() {
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&Movement{})
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&account.Account{})
		conn.DB.Unscoped().Where("id = ?", uid).Delete(&user.User{})
	})

	accRepo := account.NewRepository(conn)
	ars := &account.Account{UserID: uid, Name: "Recurring ARS", Currency: currency.ARS}
	usd := &account.Account{UserID: uid, Name: "Recurring USD", Currency: currency.USD}
	for _, a := range []*account.Account{ars, usd} {
		if err := accRepo.Insert(a); err != nil {
			t.Fatalf("insert account: %v", err)
		}
	}
	return recurringFixture{
		r:     InitRepository(conn),
		uid:   uid,
		day:   time.Date(2033, 5, 20, 0, 0, 0, 0, time.UTC),
		arsID: uint64(ars.ID),
		usdID: uint64(usd.ID),
	}
}

func (f recurringFixture) insert(t *testing.T, subID uint64, typ movementType, cur currency.Currency, date time.Time, desc *string, times int) {
	t.Helper()
	accID := f.arsID
	if cur == currency.USD {
		accID = f.usdID
	}
	amount := decimal.NewFromInt(-1000)
	if typ == Income {
		amount = amount.Neg()
	}
	movs := make([]Movement, times)
	for i := range movs {
		movs[i] = Movement{UserID: f.uid, AccountID: &accID, SubcategoryID: subID, Date: date, Type: typ, Amount: amount, Currency: cur, Description: desc}
	}
	if err := f.r.InsertBatch(movs); err != nil {
		t.Fatalf("insert movements: %v", err)
	}
}

func (f recurringFixture) top(t *testing.T, minCount, limit int) []string {
	t.Helper()
	got, err := f.r.TopRecurringDescriptions(f.uid, f.day.AddDate(0, 0, -30), f.day, minCount, limit)
	if err != nil {
		t.Fatalf("TopRecurringDescriptions: %v", err)
	}
	return got
}

func strPtr(s string) *string { return &s }

func TestTopRecurringDescriptions_OnlyCountsUserFacingARSExpensesInsideTheWindow(t *testing.T) {
	f := newRecurringFixture(t)
	conn := testConnection(t)
	food := subcategoryID(t, conn, "Alimentación", "")
	adjust := subcategoryID(t, conn, subcategory.CategorySystem, "Ajuste de saldo")
	transfer := subcategoryID(t, conn, subcategory.CategorySystem, subcategory.SubTransfer)

	f.insert(t, food, Expense, currency.ARS, f.day, strPtr("panadería"), 2)
	f.insert(t, adjust, Expense, currency.ARS, f.day, strPtr("ajuste"), 3)
	f.insert(t, transfer, Transfer, currency.ARS, f.day, strPtr("transferencia a mp"), 3)
	f.insert(t, food, Income, currency.ARS, f.day, strPtr("reintegro"), 3)
	f.insert(t, food, Expense, currency.USD, f.day, strPtr("netflix"), 3)
	f.insert(t, food, Expense, currency.ARS, f.day, strPtr("  "), 3)
	f.insert(t, food, Expense, currency.ARS, f.day, nil, 3)
	f.insert(t, food, Expense, currency.ARS, f.day.AddDate(0, 0, -40), strPtr("pollería"), 3)
	f.insert(t, food, Expense, currency.ARS, f.day, strPtr("kiosco"), 1)

	if got, want := f.top(t, 2, 3), []string{"panadería"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTopRecurringDescriptions_FoldsAccentsAndCaseIntoOneGroupShownAsItsMostFrequentSpelling(t *testing.T) {
	f := newRecurringFixture(t)
	food := subcategoryID(t, testConnection(t), "Alimentación", "")

	f.insert(t, food, Expense, currency.ARS, f.day, strPtr("Café"), 2)
	f.insert(t, food, Expense, currency.ARS, f.day, strPtr("cafe "), 1)

	if got, want := f.top(t, 3, 3), []string{"Café"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTopRecurringDescriptions_OrdersByCountThenKeyAndStopsAtLimit(t *testing.T) {
	f := newRecurringFixture(t)
	food := subcategoryID(t, testConnection(t), "Alimentación", "")

	f.insert(t, food, Expense, currency.ARS, f.day, strPtr("verdulería"), 2)
	f.insert(t, food, Expense, currency.ARS, f.day, strPtr("panadería"), 2)
	f.insert(t, food, Expense, currency.ARS, f.day, strPtr("carnicería"), 3)
	f.insert(t, food, Expense, currency.ARS, f.day, strPtr("almacén"), 2)

	if got, want := f.top(t, 2, 3), []string{"carnicería", "almacén", "panadería"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
