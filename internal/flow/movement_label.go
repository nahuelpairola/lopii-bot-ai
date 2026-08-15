package flow

import (
	"errors"

	"lopiibot.com/internal/movement"
)

// CreateErrorCopy maps a guard rejection to specific user copy, falling back
// to the generic error. Mirrors account finish's ErrAccountAlreadyExists.
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

// GuardReason maps a guard rejection to a stable log value. Mirror of
// CreateErrorCopy, which maps the same sentinels to user-facing copy.
//
// Los valores son un contrato con los logs de producción (campo `reason`):
// cambiarlos rompe cualquier búsqueda histórica, aunque se renombren los
// sentinels de Go.
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
