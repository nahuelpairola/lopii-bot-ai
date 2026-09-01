package movement

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
)

func acct(id uint64, cur currency.Currency, def bool) account.Account {
	return account.Account{Model: gorm.Model{ID: uint(id)}, UserID: 1, Currency: cur, IsDefault: def}
}

func accountsMap(accs ...account.Account) (map[uint64]account.Account, map[string]uint64) {
	byID := map[uint64]account.Account{}
	def := map[string]uint64{}
	for _, a := range accs {
		byID[uint64(a.ID)] = a
		if a.IsDefault {
			def[a.Currency.String()] = uint64(a.ID)
		}
	}
	return byID, def
}

func TestNormalize_ExpenseNegativeIncomePositive(t *testing.T) {
	byID, def := accountsMap(acct(1, currency.ARS, true))
	id := uint64(1)
	movs := []Movement{
		{Type: Expense, Amount: decimal.NewFromInt(500), Currency: currency.ARS, AccountID: &id},
		{Type: Income, Amount: decimal.NewFromInt(-700), Currency: currency.ARS, AccountID: &id},
	}
	out, err := Normalize(movs, byID, def)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !out[0].Amount.Equal(decimal.NewFromInt(-500)) {
		t.Errorf("expense = %s, want -500", out[0].Amount)
	}
	if !out[1].Amount.Equal(decimal.NewFromInt(700)) {
		t.Errorf("income = %s, want 700 (LLM sign ignored)", out[1].Amount)
	}
}

func TestNormalize_NilAccountResolvesToDefault(t *testing.T) {
	byID, def := accountsMap(acct(1, currency.ARS, true))
	movs := []Movement{{Type: Expense, Amount: decimal.NewFromInt(500), Currency: currency.ARS}}
	out, err := Normalize(movs, byID, def)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if out[0].AccountID == nil || *out[0].AccountID != 1 {
		t.Errorf("account = %v, want default 1", out[0].AccountID)
	}
}

func TestNormalize_ZeroAmountRejected(t *testing.T) {
	byID, def := accountsMap(acct(1, currency.ARS, true))
	id := uint64(1)
	movs := []Movement{{Type: Expense, Amount: decimal.Zero, Currency: currency.ARS, AccountID: &id}}
	if _, err := Normalize(movs, byID, def); !errors.Is(err, ErrZeroAmount) {
		t.Fatalf("err = %v, want ErrZeroAmount", err)
	}
}

func TestNormalize_CurrencyMismatchRejected(t *testing.T) {
	byID, def := accountsMap(acct(1, currency.ARS, true))
	id := uint64(1)
	movs := []Movement{{Type: Expense, Amount: decimal.NewFromInt(5), Currency: currency.USD, AccountID: &id}}
	if _, err := Normalize(movs, byID, def); !errors.Is(err, ErrCurrencyAccountMismatch) {
		t.Fatalf("err = %v, want ErrCurrencyAccountMismatch", err)
	}
}

func TestNormalize_NoAccountForCurrencyRejected(t *testing.T) {
	byID, def := accountsMap(acct(1, currency.ARS, true))
	movs := []Movement{{Type: Expense, Amount: decimal.NewFromInt(5), Currency: currency.USD}}
	if _, err := Normalize(movs, byID, def); !errors.Is(err, ErrNoAccountForCurrency) {
		t.Fatalf("err = %v, want ErrNoAccountForCurrency", err)
	}
}

func TestNormalize_TransferPairSumsToZeroDistinctAccounts(t *testing.T) {
	byID, def := accountsMap(acct(1, currency.ARS, true), acct(2, currency.ARS, false))
	tx := uuid.New()
	a1, a2 := uint64(1), uint64(2)
	movs := []Movement{
		{TransactionID: &tx, Type: Transfer, Amount: decimal.NewFromInt(-50), Currency: currency.ARS, AccountID: &a1},
		{TransactionID: &tx, Type: Transfer, Amount: decimal.NewFromInt(50), Currency: currency.ARS, AccountID: &a2},
	}
	if _, err := Normalize(movs, byID, def); err != nil {
		t.Fatalf("valid transfer rejected: %v", err)
	}
}

func TestNormalize_LoneTransferRejected(t *testing.T) {
	byID, def := accountsMap(acct(1, currency.ARS, true))
	a1 := uint64(1)
	movs := []Movement{{Type: Transfer, Amount: decimal.NewFromInt(-50), Currency: currency.ARS, AccountID: &a1}}
	if _, err := Normalize(movs, byID, def); !errors.Is(err, ErrTransferLeg) {
		t.Fatalf("err = %v, want ErrTransferLeg (lone transfer)", err)
	}
}

func TestNormalize_SelfTransferRejected(t *testing.T) {
	byID, def := accountsMap(acct(1, currency.ARS, true))
	tx := uuid.New()
	a1 := uint64(1)
	movs := []Movement{
		{TransactionID: &tx, Type: Transfer, Amount: decimal.NewFromInt(-50), Currency: currency.ARS, AccountID: &a1},
		{TransactionID: &tx, Type: Transfer, Amount: decimal.NewFromInt(50), Currency: currency.ARS, AccountID: &a1},
	}
	if _, err := Normalize(movs, byID, def); !errors.Is(err, ErrTransferLeg) {
		t.Fatalf("err = %v, want ErrTransferLeg (self-transfer)", err)
	}
}

func TestAssignTransactionIDs_SharedGroupOneID(t *testing.T) {
	movs := make([]Movement, 2)
	AssignTransactionIDs(movs, []string{"g1", "g1"})
	if movs[0].TransactionID == nil || movs[1].TransactionID == nil || *movs[0].TransactionID != *movs[1].TransactionID {
		t.Fatal("shared group must get one shared transaction_id")
	}
}

func TestAssignTransactionIDs_IndependentAreNil(t *testing.T) {
	movs := make([]Movement, 3)
	AssignTransactionIDs(movs, []string{"", "", ""})
	for i, m := range movs {
		if m.TransactionID != nil {
			t.Errorf("mov %d got a transaction_id, want nil (independent)", i)
		}
	}
}

func TestAssignTransactionIDs_LoneGroupIsNil(t *testing.T) {
	movs := make([]Movement, 1)
	AssignTransactionIDs(movs, []string{"g1"})
	if movs[0].TransactionID != nil {
		t.Error("a group of one must stay independent (nil)")
	}
}

func TestAssignTransactionIDs_TwoDistinctGroups(t *testing.T) {
	movs := make([]Movement, 4)
	AssignTransactionIDs(movs, []string{"a", "a", "b", "b"})
	if *movs[0].TransactionID == *movs[2].TransactionID {
		t.Error("distinct groups must get distinct transaction_ids")
	}
}

func TestCheckBalances_OutflowIntoNegativeFires(t *testing.T) {
	byID, _ := accountsMap(acct(1, currency.ARS, true))
	id := uint64(1)
	movs := []Movement{{Type: Transfer, Amount: decimal.NewFromInt(-1000000), Currency: currency.ARS, AccountID: &id}}
	short := CheckBalances(movs, map[uint64]decimal.Decimal{1: decimal.NewFromInt(200000)}, byID)
	if len(short) != 1 || !short[0].After.Equal(decimal.NewFromInt(-800000)) {
		t.Fatalf("shortfalls = %+v, want one with After=-800000", short)
	}
}

func TestCheckBalances_InflowIntoNegativeDoesNotFire(t *testing.T) {
	byID, _ := accountsMap(acct(1, currency.ARS, true))
	id := uint64(1)
	movs := []Movement{{Type: Income, Amount: decimal.NewFromInt(5000), Currency: currency.ARS, AccountID: &id}}
	if s := CheckBalances(movs, map[uint64]decimal.Decimal{1: decimal.NewFromInt(-1000)}, byID); len(s) != 0 {
		t.Fatalf("inflow into a negative account must not fire; got %+v", s)
	}
}

func TestCheckBalances_StaysNonNegativeDoesNotFire(t *testing.T) {
	byID, _ := accountsMap(acct(1, currency.ARS, true))
	id := uint64(1)
	movs := []Movement{{Type: Expense, Amount: decimal.NewFromInt(-500), Currency: currency.ARS, AccountID: &id}}
	if s := CheckBalances(movs, map[uint64]decimal.Decimal{1: decimal.NewFromInt(1000)}, byID); len(s) != 0 {
		t.Fatalf("outflow staying >= 0 must not fire; got %+v", s)
	}
}
