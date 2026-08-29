package templates

import (
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/currency"
)

// thousandsFloor is the smallest ARS amount that still rounds to 1 at
// thousands scale. Anything nonzero below it renders "<1": "0" would read as
// "no spending", and the muted dot already means that.
var (
	thousandsFloor = decimal.NewFromInt(500)
	oneThousand    = decimal.NewFromInt(1_000)
)

// FormatMoney renders an amount the way an Argentine reader expects.
// La implementación vive en internal/currency porque el mismo formato lo usan
// los mensajes de Telegram; acá queda el alias para no tocar los 11 call sites
// de la Mini App (uno de ellos es código generado por templ).
func FormatMoney(d decimal.Decimal, cur currency.Currency) string {
	return currency.FormatMoney(d, cur)
}

// FormatCompact renders a evolution cell. ARS is scaled to thousands — at peso
// magnitudes the full number costs three columns of width, and the legend
// above the table carries the scale. USD is not scaled: those amounts are
// already short, and dividing them by a thousand would render "0,1". Zero
// renders as a muted dot — an empty month should not compete for attention
// with a real number.
func FormatCompact(d decimal.Decimal, cur currency.Currency) string {
	v := d.Abs()
	switch {
	case v.IsZero():
		return "·"
	case cur == currency.USD:
		return groupThousands(v.StringFixed(0))
	case v.LessThan(thousandsFloor):
		return "<1"
	default:
		return groupThousands(v.Div(oneThousand).StringFixed(0))
	}
}

// ScaleLabel names what FormatCompact did to the numbers. It travels without a
// separator: the legend composes the punctuation and drops the whole segment
// when this is empty. Empty for USD, which is not scaled.
func ScaleLabel(cur currency.Currency) string {
	if cur == currency.USD {
		return ""
	}
	return "en miles de $"
}

func groupThousands(s string) string { return currency.GroupThousands(s) }
