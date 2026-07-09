package messaging

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"gorm.io/gorm"
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
	movs := []movement.Movement{
		{Type: movement.Expense, Amount: decimal.NewFromInt(500), Currency: currency.ARS, AccountID: &id},
		{Type: movement.Income, Amount: decimal.NewFromInt(-700), Currency: currency.ARS, AccountID: &id},
	}
	out, err := normalizeMovements(movs, byID, def)
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
	movs := []movement.Movement{{Type: movement.Expense, Amount: decimal.NewFromInt(500), Currency: currency.ARS}}
	out, err := normalizeMovements(movs, byID, def)
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
	movs := []movement.Movement{{Type: movement.Expense, Amount: decimal.Zero, Currency: currency.ARS, AccountID: &id}}
	if _, err := normalizeMovements(movs, byID, def); !errors.Is(err, errZeroAmount) {
		t.Fatalf("err = %v, want errZeroAmount", err)
	}
}

func TestNormalize_CurrencyMismatchRejected(t *testing.T) {
	byID, def := accountsMap(acct(1, currency.ARS, true))
	id := uint64(1)
	movs := []movement.Movement{{Type: movement.Expense, Amount: decimal.NewFromInt(5), Currency: currency.USD, AccountID: &id}}
	if _, err := normalizeMovements(movs, byID, def); !errors.Is(err, errCurrencyAccountMismatch) {
		t.Fatalf("err = %v, want errCurrencyAccountMismatch", err)
	}
}

func TestNormalize_NoAccountForCurrencyRejected(t *testing.T) {
	byID, def := accountsMap(acct(1, currency.ARS, true))
	movs := []movement.Movement{{Type: movement.Expense, Amount: decimal.NewFromInt(5), Currency: currency.USD}}
	if _, err := normalizeMovements(movs, byID, def); !errors.Is(err, errNoAccountForCurrency) {
		t.Fatalf("err = %v, want errNoAccountForCurrency", err)
	}
}

func TestNormalize_TransferPairSumsToZeroDistinctAccounts(t *testing.T) {
	byID, def := accountsMap(acct(1, currency.ARS, true), acct(2, currency.ARS, false))
	tx := uuid.New()
	a1, a2 := uint64(1), uint64(2)
	movs := []movement.Movement{
		{TransactionID: &tx, Type: movement.Transfer, Amount: decimal.NewFromInt(-50), Currency: currency.ARS, AccountID: &a1},
		{TransactionID: &tx, Type: movement.Transfer, Amount: decimal.NewFromInt(50), Currency: currency.ARS, AccountID: &a2},
	}
	if _, err := normalizeMovements(movs, byID, def); err != nil {
		t.Fatalf("valid transfer rejected: %v", err)
	}
}

func TestNormalize_LoneTransferRejected(t *testing.T) {
	byID, def := accountsMap(acct(1, currency.ARS, true))
	a1 := uint64(1)
	movs := []movement.Movement{{Type: movement.Transfer, Amount: decimal.NewFromInt(-50), Currency: currency.ARS, AccountID: &a1}}
	if _, err := normalizeMovements(movs, byID, def); !errors.Is(err, errTransferLeg) {
		t.Fatalf("err = %v, want errTransferLeg (lone transfer)", err)
	}
}

func TestNormalize_SelfTransferRejected(t *testing.T) {
	byID, def := accountsMap(acct(1, currency.ARS, true))
	tx := uuid.New()
	a1 := uint64(1)
	movs := []movement.Movement{
		{TransactionID: &tx, Type: movement.Transfer, Amount: decimal.NewFromInt(-50), Currency: currency.ARS, AccountID: &a1},
		{TransactionID: &tx, Type: movement.Transfer, Amount: decimal.NewFromInt(50), Currency: currency.ARS, AccountID: &a1},
	}
	if _, err := normalizeMovements(movs, byID, def); !errors.Is(err, errTransferLeg) {
		t.Fatalf("err = %v, want errTransferLeg (self-transfer)", err)
	}
}

func TestAssignTransactionIDs_SharedGroupOneID(t *testing.T) {
	movs := make([]movement.Movement, 2)
	assignTransactionIDs(movs, []string{"g1", "g1"})
	if movs[0].TransactionID == nil || movs[1].TransactionID == nil || *movs[0].TransactionID != *movs[1].TransactionID {
		t.Fatal("shared group must get one shared transaction_id")
	}
}

func TestAssignTransactionIDs_IndependentAreNil(t *testing.T) {
	movs := make([]movement.Movement, 3)
	assignTransactionIDs(movs, []string{"", "", ""}) // pan, med, carne
	for i, m := range movs {
		if m.TransactionID != nil {
			t.Errorf("mov %d got a transaction_id, want nil (independent)", i)
		}
	}
}

func TestAssignTransactionIDs_LoneGroupIsNil(t *testing.T) {
	movs := make([]movement.Movement, 1)
	assignTransactionIDs(movs, []string{"g1"}) // only one member → not a group
	if movs[0].TransactionID != nil {
		t.Error("a group of one must stay independent (nil)")
	}
}

func TestAssignTransactionIDs_TwoDistinctGroups(t *testing.T) {
	movs := make([]movement.Movement, 4) // pasé 10 al banco y 20 a MP
	assignTransactionIDs(movs, []string{"a", "a", "b", "b"})
	if *movs[0].TransactionID == *movs[2].TransactionID {
		t.Error("distinct groups must get distinct transaction_ids")
	}
}

func TestCheckBalances_OutflowIntoNegativeFires(t *testing.T) {
	byID, _ := accountsMap(acct(1, currency.ARS, true))
	id := uint64(1)
	movs := []movement.Movement{{Type: movement.Transfer, Amount: decimal.NewFromInt(-1000000), Currency: currency.ARS, AccountID: &id}}
	short := checkResultingBalances(movs, map[uint64]decimal.Decimal{1: decimal.NewFromInt(200000)}, byID)
	if len(short) != 1 || !short[0].After.Equal(decimal.NewFromInt(-800000)) {
		t.Fatalf("shortfalls = %+v, want one with After=-800000", short)
	}
}

func TestCheckBalances_InflowIntoNegativeDoesNotFire(t *testing.T) {
	byID, _ := accountsMap(acct(1, currency.ARS, true))
	id := uint64(1)
	movs := []movement.Movement{{Type: movement.Income, Amount: decimal.NewFromInt(5000), Currency: currency.ARS, AccountID: &id}}
	if s := checkResultingBalances(movs, map[uint64]decimal.Decimal{1: decimal.NewFromInt(-1000)}, byID); len(s) != 0 {
		t.Fatalf("inflow into a negative account must not fire; got %+v", s)
	}
}

func TestCheckBalances_StaysNonNegativeDoesNotFire(t *testing.T) {
	byID, _ := accountsMap(acct(1, currency.ARS, true))
	id := uint64(1)
	movs := []movement.Movement{{Type: movement.Expense, Amount: decimal.NewFromInt(-500), Currency: currency.ARS, AccountID: &id}}
	if s := checkResultingBalances(movs, map[uint64]decimal.Decimal{1: decimal.NewFromInt(1000)}, byID); len(s) != 0 {
		t.Fatalf("outflow staying >= 0 must not fire; got %+v", s)
	}
}

func TestCorrectionIsDeletion(t *testing.T) {
	cases := []struct {
		name string
		rows []movementRow
		want bool
	}{
		{"empty set is not a deletion", nil, false},
		{"single zero row deletes", []movementRow{{Amount: "0"}}, true},
		{"zero with decimals deletes", []movementRow{{Amount: "0.00"}}, true},
		{"non-zero is a real correction", []movementRow{{Amount: "600"}}, false},
		{"mixed zero and non-zero is not a deletion", []movementRow{{Amount: "0"}, {Amount: "500"}}, false},
		{"unparseable amount is not a deletion", []movementRow{{Amount: ""}}, false},
	}
	for _, tc := range cases {
		if got := correctionIsDeletion(tc.rows); got != tc.want {
			t.Errorf("%s: correctionIsDeletion = %v, want %v", tc.name, got, tc.want)
		}
	}
}
