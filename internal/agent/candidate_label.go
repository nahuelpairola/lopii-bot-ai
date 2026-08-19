package agent

import (
	"strings"

	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

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
