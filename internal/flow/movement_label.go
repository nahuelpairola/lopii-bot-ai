package flow

import (
	"errors"

	"lopiibot.com/internal/movement"
)

func CreateErrorCopy(err error) string {
	switch {
	case errors.Is(err, movement.ErrZeroAmount):
		return MsgAmountUnclear
	case errors.Is(err, movement.ErrCurrencyAccountMismatch):
		return MsgCurrencyMismatch
	case errors.Is(err, movement.ErrNoAccountForCurrency):
		return MsgNoAccountCurrency
	case errors.Is(err, movement.ErrTransferLeg):
		return MsgMovementMalformed
	default:
		return MsgCouldNotSave("tu movimiento")
	}
}

func GuardReason(err error) string {
	switch {
	case errors.Is(err, movement.ErrZeroAmount):
		return "zero_amount"
	case errors.Is(err, movement.ErrCurrencyAccountMismatch):
		return "currency_account_mismatch"
	case errors.Is(err, movement.ErrNoAccountForCurrency):
		return "no_account_for_currency"
	case errors.Is(err, movement.ErrTransferLeg):
		return "malformed_transfer"
	default:
		return "other"
	}
}
