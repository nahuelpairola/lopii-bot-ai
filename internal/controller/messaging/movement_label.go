package messaging

import (
	"errors"
	"strings"

	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

// createErrorCopy maps a guard rejection to specific user copy, falling back
// to the generic error. Mirrors finishAccountCreateFlow's ErrAccountAlreadyExists.
func createErrorCopy(err error) string {
	switch {
	case errors.Is(err, movement.ErrZeroAmount):
		return msgAmountUnclear
	case errors.Is(err, movement.ErrCurrencyAccountMismatch):
		return msgCurrencyMismatch
	case errors.Is(err, movement.ErrNoAccountForCurrency):
		return msgNoAccountCurrency
	case errors.Is(err, movement.ErrTransferLeg):
		return msgMovementMalformed
	default:
		return msgCouldNotSave("tu movimiento")
	}
}

// guardReason maps a guard rejection to a stable log value. Mirror of
// createErrorCopy, which maps the same sentinels to user-facing copy.
//
// Los valores son un contrato con los logs de producción (campo `reason`):
// cambiarlos rompe cualquier búsqueda histórica, aunque se renombren los
// sentinels de Go.
func guardReason(err error) string {
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

// candidateLabel builds the short display line shown per option in both
// UPDATE's and DELETE's ambiguous-candidate pickers.
//
// Formato: "🔴 Pan · $2.000 · hoy". Telegram corta los labels largos, así que
// cada parte se gana el lugar: qué fue, cuánto, cuándo. El monto va en formato
// argentino (antes salía "2000 ARS", y un saldo grande como "21528105 ARS"),
// la fecha en relativo (antes "2026-07-27"), y siempre hay un nombre: sin
// descripción caía a "· ·", que no le dice nada a nadie.
//
// El monto va SIEMPRE en positivo: los movimientos vienen de la DB con el signo
// contable, y ese signo no escapa de storage — la dirección la da el tipo.
func candidateLabel(g transactionGroup) string {
	if len(g.Movements) == 0 {
		return "?"
	}
	m := g.Movements[0]

	name := ""
	if m.Description != nil {
		name = strings.TrimSpace(*m.Description)
	}
	if name == "" && m.Subcategory != nil {
		name = m.Subcategory.Subcategory
	}

	return movement.IconForType(m.Type) + " " + name +
		" · " + currency.FormatMoney(m.Amount.Abs(), m.Currency) +
		" · " + movement.RelativeDate(m.Date)
}
