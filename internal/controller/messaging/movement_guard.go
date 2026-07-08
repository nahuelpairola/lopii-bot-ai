package messaging

import (
	"errors"

	"github.com/shopspring/decimal"
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
