package currency

import (
	"strings"

	"github.com/shopspring/decimal"
)

func FormatMoney(d decimal.Decimal, cur Currency) string {
	sign := ""
	if d.IsNegative() {
		sign = "-"
	}
	abs := d.Abs()
	if cur == USD {
		return sign + "US$" + GroupThousands(abs.StringFixed(2))
	}
	return sign + "$" + GroupThousands(abs.StringFixed(0))
}

func GroupThousands(s string) string {
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
