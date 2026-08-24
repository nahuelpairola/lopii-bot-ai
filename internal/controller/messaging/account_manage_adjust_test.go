package messaging

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

func adjustData(newTotal string) conversation.Data {
	return conversation.Data{
		conversation.UserIDKey: uint64(1),
		"operation":            "adjust",
		"account_id":           "5",
		"account_name":         "Wallet",
		"account_currency":     "ARS",
		"new_total":            newTotal,
	}
}

func newAdjustController(currentBalance string) (*controller, *fakeMovementRepoFull, *fakeMetricRepo) {
	sub := &subcategory.Subcategory{}
	sub.ID = 9
	movRepo := &fakeMovementRepoFull{balances: map[uint64]string{5: currentBalance}}
	metrics := &fakeMetricRepo{}
	// La cuenta 5 tiene que existir en el repo: finishAccountAdjust la relee para
	// que el guard valide la moneda del movimiento contra la de la DB, y no
	// contra la que dice la Data del flujo.
	acc5 := acct(5, currency.ARS, true)
	c := &controller{
		movements:     movRepo,
		metrics:       metrics,
		accounts:      &fakeAccountRepoFull{byID: map[uint64]*account.Account{5: &acc5}},
		subcategories: &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{"Sistema|Ajuste de saldo": sub}},
	}
	return c, movRepo, metrics
}

func onlyInserted(t *testing.T, movRepo *fakeMovementRepoFull) movement.Movement {
	t.Helper()
	if len(movRepo.inserted) != 1 {
		t.Fatalf("expected exactly 1 movement inserted, got %d", len(movRepo.inserted))
	}
	return movRepo.inserted[0]
}

// (a) positive delta → Income +2000, correct subcategory/account/currency.
func TestFinishAccountAdjust_PositiveDelta(t *testing.T) {
	c, movRepo, metrics := newAdjustController("50000")
	c.finishAccountAdjust(context.Background(), &messenger.FakeChat{}, adjustData("52000"))

	m := onlyInserted(t, movRepo)
	if m.Type != movement.Income || !m.Amount.Equal(decimal.NewFromInt(2000)) {
		t.Errorf("movement = %s %s, want Income +2000", m.Type, m.Amount)
	}
	if m.SubcategoryID != 9 || m.AccountID == nil || *m.AccountID != 5 || m.Currency.String() != "ARS" {
		t.Errorf("movement fields = sub %d acct %v cur %s, want 9/5/ARS", m.SubcategoryID, m.AccountID, m.Currency)
	}
	if !resolvedContains(metrics, outcomeAccountAdjusted) {
		t.Errorf("outcomes = %v, want %q", metrics.resolved, outcomeAccountAdjusted)
	}
}

// (b) negative delta → Expense stored with negative sign.
func TestFinishAccountAdjust_NegativeDelta(t *testing.T) {
	c, movRepo, _ := newAdjustController("50000")
	c.finishAccountAdjust(context.Background(), &messenger.FakeChat{}, adjustData("48000"))

	m := onlyInserted(t, movRepo)
	if m.Type != movement.Expense || !m.Amount.Equal(decimal.NewFromInt(-2000)) {
		t.Errorf("movement = %s %s, want Expense -2000", m.Type, m.Amount)
	}
}

// (c) already at that total → no insert, still resolves.
func TestFinishAccountAdjust_NoChange(t *testing.T) {
	c, movRepo, metrics := newAdjustController("50000")
	c.finishAccountAdjust(context.Background(), &messenger.FakeChat{}, adjustData("50000"))

	if len(movRepo.inserted) != 0 {
		t.Errorf("no change should insert nothing, got %d movements", len(movRepo.inserted))
	}
	if !resolvedContains(metrics, outcomeAccountAdjusted) {
		t.Errorf("outcomes = %v, want %q even on no-op", metrics.resolved, outcomeAccountAdjusted)
	}
}

// (d) "dejar en cero": 50000 → 0 → Expense -50000.
func TestFinishAccountAdjust_ToZero(t *testing.T) {
	c, movRepo, _ := newAdjustController("50000")
	c.finishAccountAdjust(context.Background(), &messenger.FakeChat{}, adjustData("0"))

	m := onlyInserted(t, movRepo)
	if m.Type != movement.Expense || !m.Amount.Equal(decimal.NewFromInt(-50000)) {
		t.Errorf("movement = %s %s, want Expense -50000", m.Type, m.Amount)
	}
}

// (e) account with no movements: balance 0 → 30000 → Income +30000.
func TestFinishAccountAdjust_NoMovements(t *testing.T) {
	c, movRepo, _ := newAdjustController("0")
	c.finishAccountAdjust(context.Background(), &messenger.FakeChat{}, adjustData("30000"))

	m := onlyInserted(t, movRepo)
	if m.Type != movement.Income || !m.Amount.Equal(decimal.NewFromInt(30000)) {
		t.Errorf("movement = %s %s, want Income +30000", m.Type, m.Amount)
	}
}

// (f) negative current balance (overdraft): -1000 → 500 → Income +1500.
func TestFinishAccountAdjust_NegativeCurrentBalance(t *testing.T) {
	c, movRepo, _ := newAdjustController("-1000")
	c.finishAccountAdjust(context.Background(), &messenger.FakeChat{}, adjustData("500"))

	m := onlyInserted(t, movRepo)
	if m.Type != movement.Income || !m.Amount.Equal(decimal.NewFromInt(1500)) {
		t.Errorf("movement = %s %s, want Income +1500", m.Type, m.Amount)
	}
}

// (g) decimal precision: 100.25 → 100.50 → Income +0.25 (exact, never float).
func TestFinishAccountAdjust_DecimalPrecision(t *testing.T) {
	c, movRepo, _ := newAdjustController("100.25")
	c.finishAccountAdjust(context.Background(), &messenger.FakeChat{}, adjustData("100.50"))

	m := onlyInserted(t, movRepo)
	want, _ := decimal.NewFromString("0.25")
	if m.Type != movement.Income || !m.Amount.Equal(want) {
		t.Errorf("movement = %s %s, want Income +0.25", m.Type, m.Amount)
	}
}

// TestFinishAccountAdjust_CurrencyMismatchRejected cubre lo que este camino gana
// al pasar por el guard: si la moneda que arrastra la Data del flujo no es la de
// la cuenta en la DB, el ajuste NO se escribe.
//
// Sin el guard el movimiento entraba igual, en una moneda distinta a la de su
// cuenta, y el balance quedaba corrupto en silencio: el saldo es SUM(amount) y no
// mira la moneda de cada fila, así que sumaría USD contra pesos sin chistar.
func TestFinishAccountAdjust_CurrencyMismatchRejected(t *testing.T) {
	c, movRepo, _ := newAdjustController("50000") // la cuenta 5 es ARS
	data := adjustData("52000")
	data["account_currency"] = "USD" // la Data dice otra cosa

	c.finishAccountAdjust(context.Background(), &messenger.FakeChat{}, data)

	if len(movRepo.inserted) != 0 {
		t.Fatalf("no se debe escribir nada con la moneda equivocada, se escribió %+v", movRepo.inserted)
	}
}
