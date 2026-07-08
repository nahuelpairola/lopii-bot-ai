package messaging

import (
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/movement"
)

// Guard rejection sentinels. Call sites errors.Is-map them to specific copy
// (see finishAccountCreateFlow's ErrAccountAlreadyExists handling).
var (
	errZeroAmount              = errors.New("movement amount is zero")
	errCurrencyAccountMismatch = errors.New("movement currency differs from its account")
	errTransferLeg             = errors.New("malformed transfer (legs not a distinct 2-leg group)")
	errNoAccountForCurrency    = errors.New("no account exists for this currency")
)

// accountShortfall is one account a CREATE would drive into/deeper into negative.
type accountShortfall struct {
	AccountID uint64
	Name      string
	Currency  string
	After     decimal.Decimal
}

// insufficientFunds is returned by the insert path when the CREATE would leave
// one or more accounts negative; the caller diverts to the confirm gate.
type insufficientFunds struct {
	shortfalls []accountShortfall
}

func (e *insufficientFunds) Error() string { return "insufficient funds" }

// normalizeMovements enforces the money-model invariants on a fully-built,
// transaction_id-assigned movement set. It mutates in place: expense→negative,
// income→positive (LLM sign ignored); a nil account resolves to the currency's
// default; and it rejects zero amounts, currency/account mismatches, and
// malformed transfer groups. Pure (no repo/DB) — accountsByID and
// defaultByCurrency are supplied by the caller.
func normalizeMovements(movs []movement.Movement, accountsByID map[uint64]account.Account, defaultByCurrency map[string]uint64) ([]movement.Movement, error) {
	for i := range movs {
		m := &movs[i]
		if m.Amount.IsZero() {
			return nil, errZeroAmount
		}
		if m.AccountID == nil {
			id, ok := defaultByCurrency[m.Currency.String()]
			if !ok {
				return nil, errNoAccountForCurrency
			}
			m.AccountID = &id
		}
		acc, ok := accountsByID[*m.AccountID]
		if !ok || acc.Currency != m.Currency {
			return nil, errCurrencyAccountMismatch
		}
		switch m.Type {
		case movement.Expense:
			m.Amount = m.Amount.Abs().Neg()
		case movement.Income:
			m.Amount = m.Amount.Abs()
		}
		// transfer: keep the LLM-classified sign (out negative / in positive).
	}
	if err := validateTransferGroups(movs); err != nil {
		return nil, err
	}
	return movs, nil
}

// validateTransferGroups checks every transfer belongs to a 2-leg,
// same-transaction_id, distinct-account group; same-currency groups sum to 0.
func validateTransferGroups(movs []movement.Movement) error {
	type leg struct {
		accounts map[uint64]bool
		count    int
		sum      decimal.Decimal
		oneCur   bool
		currency string
	}
	groups := map[uuid.UUID]*leg{}
	for _, m := range movs {
		if m.Type != movement.Transfer {
			continue
		}
		if m.TransactionID == nil || m.AccountID == nil {
			return errTransferLeg // a transfer with no group or no account is malformed
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
			return errTransferLeg
		}
		if g.oneCur && !g.sum.IsZero() {
			return errTransferLeg
		}
	}
	return nil
}

// assignTransactionIDs groups movements by the LLM-supplied group tag: a
// non-empty group with 2+ members shares one fresh transaction_id; a lone or
// empty group stays nil (independent). Replaces the old len(rows)>1 heuristic
// that wrongly grouped independent movements.
func assignTransactionIDs(movs []movement.Movement, groups []string) {
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

// checkResultingBalances returns the accounts a movement set would drive into
// or deeper into negative: after = before + Σdeltas, firing only when
// after < 0 AND after < before (a net outflow that leaves it negative — an
// inflow, or an outflow that stays >= 0, never fires; an already-negative
// account isn't nagged unless the movement makes it worse).
func checkResultingBalances(movs []movement.Movement, balances map[uint64]decimal.Decimal, accountsByID map[uint64]account.Account) []accountShortfall {
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
	var out []accountShortfall
	for id, delta := range deltas {
		before, ok := balances[id]
		if !ok {
			before = decimal.Zero
		}
		after := before.Add(delta)
		if after.IsNegative() && after.LessThan(before) {
			acc := accountsByID[id]
			out = append(out, accountShortfall{AccountID: id, Name: acc.Name, Currency: acc.Currency.String(), After: after})
		}
	}
	return out
}
