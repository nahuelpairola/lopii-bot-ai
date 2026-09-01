package agent

import (
	"strings"

	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

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
