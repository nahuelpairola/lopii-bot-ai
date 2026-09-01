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

func TestFinishAccountAdjust_NegativeDelta(t *testing.T) {
	c, movRepo, _ := newAdjustController("50000")
	c.finishAccountAdjust(context.Background(), &messenger.FakeChat{}, adjustData("48000"))

	m := onlyInserted(t, movRepo)
	if m.Type != movement.Expense || !m.Amount.Equal(decimal.NewFromInt(-2000)) {
		t.Errorf("movement = %s %s, want Expense -2000", m.Type, m.Amount)
	}
}

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

func TestFinishAccountAdjust_ToZero(t *testing.T) {
	c, movRepo, _ := newAdjustController("50000")
	c.finishAccountAdjust(context.Background(), &messenger.FakeChat{}, adjustData("0"))

	m := onlyInserted(t, movRepo)
	if m.Type != movement.Expense || !m.Amount.Equal(decimal.NewFromInt(-50000)) {
		t.Errorf("movement = %s %s, want Expense -50000", m.Type, m.Amount)
	}
}

func TestFinishAccountAdjust_NoMovements(t *testing.T) {
	c, movRepo, _ := newAdjustController("0")
	c.finishAccountAdjust(context.Background(), &messenger.FakeChat{}, adjustData("30000"))

	m := onlyInserted(t, movRepo)
	if m.Type != movement.Income || !m.Amount.Equal(decimal.NewFromInt(30000)) {
		t.Errorf("movement = %s %s, want Income +30000", m.Type, m.Amount)
	}
}

func TestFinishAccountAdjust_NegativeCurrentBalance(t *testing.T) {
	c, movRepo, _ := newAdjustController("-1000")
	c.finishAccountAdjust(context.Background(), &messenger.FakeChat{}, adjustData("500"))

	m := onlyInserted(t, movRepo)
	if m.Type != movement.Income || !m.Amount.Equal(decimal.NewFromInt(1500)) {
		t.Errorf("movement = %s %s, want Income +1500", m.Type, m.Amount)
	}
}

func TestFinishAccountAdjust_DecimalPrecision(t *testing.T) {
	c, movRepo, _ := newAdjustController("100.25")
	c.finishAccountAdjust(context.Background(), &messenger.FakeChat{}, adjustData("100.50"))

	m := onlyInserted(t, movRepo)
	want, _ := decimal.NewFromString("0.25")
	if m.Type != movement.Income || !m.Amount.Equal(want) {
		t.Errorf("movement = %s %s, want Income +0.25", m.Type, m.Amount)
	}
}

func TestFinishAccountAdjust_CurrencyMismatchRejected(t *testing.T) {
	c, movRepo, _ := newAdjustController("50000")
	data := adjustData("52000")
	data["account_currency"] = "USD"

	c.finishAccountAdjust(context.Background(), &messenger.FakeChat{}, data)

	if len(movRepo.inserted) != 0 {
		t.Fatalf("no se debe escribir nada con la moneda equivocada, se escribió %+v", movRepo.inserted)
	}
}
