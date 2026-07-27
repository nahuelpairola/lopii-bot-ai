package currency

import (
	"strings"

	"github.com/shopspring/decimal"
)

// FormatMoney renders an amount the way an Argentine reader expects: "." for
// thousands, "," for decimals. ARS drops the cents — at peso scale they are
// noise. The sign goes outside the symbol ("-$560.000").
//
// Vive acá y no en un paquete de presentación porque lo necesitan tanto la Mini
// App como los mensajes de Telegram: el formato de un monto es propio de la
// moneda, no del canal que lo muestra.
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

// GroupThousands turns a plain decimal string ("1234567.89") into AR format
// ("1.234.567,89").
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
