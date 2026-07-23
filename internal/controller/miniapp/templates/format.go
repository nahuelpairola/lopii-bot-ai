package templates

import (
	"strings"

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

// FormatMoney renders an amount the way an Argentine reader expects: "." for
// thousands, "," for decimals. ARS drops the cents — at peso scale they are
// noise on a dashboard. The sign goes outside the symbol ("-$560.000").
func FormatMoney(d decimal.Decimal, cur currency.Currency) string {
	sign := ""
	if d.IsNegative() {
		sign = "-"
	}
	abs := d.Abs()
	if cur == currency.USD {
		return sign + "US$" + groupThousands(abs.StringFixed(2))
	}
	return sign + "$" + groupThousands(abs.StringFixed(0))
}

// FormatCompact renders a evolution cell. ARS is scaled to thousands — at peso
// magnitudes the full number costs three columns of width, and the table's
// caption carries the scale. USD is not scaled: those amounts are already
// short, and dividing them by a thousand would render "0,1". Zero renders as a
// muted dot — an empty month should not compete for attention with a real
// number.
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

// ScaleNote is the caption suffix that tells the reader what FormatCompact did
// to the numbers. It carries its own separator and is empty when nothing was
// scaled, so the caption can concatenate it without a conditional.
func ScaleNote(cur currency.Currency) string {
	if cur == currency.USD {
		return ""
	}
	return " · en miles de $"
}

// groupThousands turns a plain decimal string ("1234567.89") into AR format
// ("1.234.567,89").
func groupThousands(s string) string {
	intPart, frac, hasFrac := strings.Cut(s, ".")
	var b strings.Builder
	for i := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteByte(intPart[i])
	}
	if hasFrac {
		return b.String() + "," + frac
	}
	return b.String()
}
