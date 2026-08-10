package messaging

import (
	"errors"
	"strings"
	"time"

	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

// createErrorCopy maps a guard rejection to specific user copy, falling back
// to the generic error. Mirrors finishAccountCreateFlow's ErrAccountAlreadyExists.
func createErrorCopy(err error) string {
	switch {
	case errors.Is(err, errZeroAmount):
		return msgAmountUnclear
	case errors.Is(err, errCurrencyAccountMismatch):
		return msgCurrencyMismatch
	case errors.Is(err, errNoAccountForCurrency):
		return msgNoAccountCurrency
	case errors.Is(err, errTransferLeg):
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
	case errors.Is(err, errZeroAmount):
		return "zero_amount"
	case errors.Is(err, errCurrencyAccountMismatch):
		return "currency_account_mismatch"
	case errors.Is(err, errNoAccountForCurrency):
		return "no_account_for_currency"
	case errors.Is(err, errTransferLeg):
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
// descripción ni merchant caía a "· ·", que no le dice nada a nadie.
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
	if name == "" && m.Merchant != nil {
		name = strings.TrimSpace(*m.Merchant)
	}
	if name == "" && m.Subcategory != nil {
		name = m.Subcategory.Subcategory
	}

	return movement.IconForType(m.Type) + " " + name +
		" · " + currency.FormatMoney(m.Amount.Abs(), m.Currency) +
		" · " + relativeDate(m.Date)
}

// relativeDate rinde una fecha como la diría una persona. Sin año: los
// candidatos salen de una ventana de días, no de meses.
//
// Compara DÍAS CALENDARIO, no instantes. La fecha de un movimiento es una fecha
// civil que entra por time.Parse("2006-01-02"), o sea medianoche UTC, mientras
// que la medianoche argentina son las 03:00 UTC: comparadas como instantes, todo
// lo cargado hoy caía 3 horas antes del corte y salía "ayer" (visto en Telegram).
// Convertir la fecha a ART tampoco sirve — la corre un día para atrás.
func relativeDate(d time.Time) string {
	day := civilDay(d)
	today := todayCivil()
	switch {
	case !day.Before(today):
		return "hoy"
	case !day.Before(today.AddDate(0, 0, -1)):
		return "ayer"
	default:
		return d.Format("02/01")
	}
}

// civilDay descarta la hora y la zona: deja solo el día del calendario, anclado
// a UTC para que dos fechas se puedan comparar entre sí sin que el huso mueva
// ninguna de las dos.
func civilDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// todayCivil es el día de HOY para el usuario, no para el server. El server
// corre en UTC, así que entre las 21:00 y las 24:00 ART time.Now() ya está en
// el día siguiente: un gasto cargado 21:50 se guardaba con la fecha de mañana
// (visto en producción). Todo lo que signifique "hoy" —la fecha por defecto de
// un movimiento, el "Hoy es" de los prompts, el corte de relativeDate— sale de
// acá y de ningún otro lado.
func todayCivil() time.Time {
	return civilDay(time.Now().In(constants.ArgentinaZone))
}
