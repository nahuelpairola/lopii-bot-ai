package movement

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
)

var (
	ErrZeroAmount              = errors.New("movement amount is zero")
	ErrCurrencyAccountMismatch = errors.New("movement currency differs from its account")
	ErrTransferLeg             = errors.New("malformed transfer (legs not a distinct 2-leg group)")
	ErrNoAccountForCurrency    = errors.New("no account exists for this currency")
)

type AccountShortfall struct {
	AccountID uint64
	Name      string
	Currency  string
	After     decimal.Decimal
}

func Normalize(movs []Movement, accountsByID map[uint64]account.Account, defaultByCurrency map[string]uint64) ([]Movement, error) {
	for i := range movs {
		m := &movs[i]
		if m.Amount.IsZero() {
			return nil, ErrZeroAmount
		}
		if m.AccountID == nil {
			id, ok := defaultByCurrency[m.Currency.String()]
			if !ok {
				return nil, ErrNoAccountForCurrency
			}
			m.AccountID = &id
		}
		acc, ok := accountsByID[*m.AccountID]
		if !ok || acc.Currency != m.Currency {
			return nil, ErrCurrencyAccountMismatch
		}
		switch m.Type {
		case Expense:
			m.Amount = m.Amount.Abs().Neg()
		case Income:
			m.Amount = m.Amount.Abs()
		}
	}
	if err := validateTransferGroups(movs); err != nil {
		return nil, err
	}
	return movs, nil
}

func validateTransferGroups(movs []Movement) error {
	type leg struct {
		accounts map[uint64]bool
		count    int
		sum      decimal.Decimal
		oneCur   bool
		currency string
	}
	groups := map[uuid.UUID]*leg{}
	for _, m := range movs {
		if m.Type != Transfer {
			continue
		}
		if m.TransactionID == nil || m.AccountID == nil {
			return fmt.Errorf("%w: leg sin group o sin cuenta", ErrTransferLeg)
		}
		g := groups[*m.TransactionID]
		if g == nil {
			g = &leg{accounts: map[uint64]bool{}, sum: decimal.Zero, oneCur: true, currency: m.Currency.String()}
			groups[*m.TransactionID] = g
		}
		if g.currency != m.Currency.String() {
			g.oneCur = false
		}
		g.accounts[*m.AccountID] = true
		g.count++
		g.sum = g.sum.Add(m.Amount)
	}
	for _, g := range groups {
		if g.count != 2 || len(g.accounts) != 2 {
			return fmt.Errorf("%w: %d legs sobre %d cuentas distintas, se esperaban 2 y 2", ErrTransferLeg, g.count, len(g.accounts))
		}
		if g.oneCur && !g.sum.IsZero() {
			return fmt.Errorf("%w: las 2 piernas en %s no se cancelan (suma %s), falta el signo opuesto", ErrTransferLeg, g.currency, g.sum)
		}
	}
	return nil
}

func AssignTransactionIDs(movs []Movement, groups []string) {
	counts := map[string]int{}
	for _, g := range groups {
		if g != "" {
			counts[g]++
		}
	}
	ids := map[string]*uuid.UUID{}
	for i := range movs {
		g := groups[i]
		if g == "" || counts[g] < 2 {
			continue
		}
		if ids[g] == nil {
			id := uuid.New()
			ids[g] = &id
		}
		movs[i].TransactionID = ids[g]
	}
}

func CheckBalances(movs []Movement, balances map[uint64]decimal.Decimal, accountsByID map[uint64]account.Account) []AccountShortfall {
	deltas := map[uint64]decimal.Decimal{}
	for _, m := range movs {
		if m.AccountID == nil {
			continue
		}
		d, ok := deltas[*m.AccountID]
		if !ok {
			d = decimal.Zero
		}
		deltas[*m.AccountID] = d.Add(m.Amount)
	}
	var out []AccountShortfall
	for id, delta := range deltas {
		before, ok := balances[id]
		if !ok {
			before = decimal.Zero
		}
		after := before.Add(delta)
		if after.IsNegative() && after.LessThan(before) {
			acc := accountsByID[id]
			out = append(out, AccountShortfall{AccountID: id, Name: acc.Name, Currency: acc.Currency.String(), After: after})
		}
	}
	return out
}
