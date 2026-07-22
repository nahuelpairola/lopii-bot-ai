package templates

import (
	"strings"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/currency"
)

// millionCutoff is where FormatCompact switches to "M". It sits below 1M on
// purpose: 999999 rounded at thousands scale would render "1000k".
var (
	millionCutoff  = decimal.NewFromInt(999_500)
	thousandCutoff = decimal.NewFromInt(1_000)
	oneMillion     = decimal.NewFromInt(1_000_000)
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

// FormatCompact renders a matrix cell: short enough for a phone column. Zero
// renders as a muted dot — an empty month should not compete for attention
// with a real number.
func FormatCompact(d decimal.Decimal) string {
	v := d.Abs()
	switch {
	case v.IsZero():
		return "·"
	case v.GreaterThanOrEqual(millionCutoff):
		return groupThousands(v.Div(oneMillion).StringFixed(1)) + "M"
	case v.GreaterThanOrEqual(thousandCutoff):
		return v.Div(oneThousand).StringFixed(0) + "k"
	default:
		return v.StringFixed(0)
	}
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
