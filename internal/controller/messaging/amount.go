package messaging

import (
	"strings"

	"github.com/shopspring/decimal"
)

// parseARAmount parses a user-typed money string into a decimal, tolerant of
// how a real person writes it: currency symbols ($), internal spaces, Argentine
// convention (comma = decimal, dots = thousands) AND plain intl dot-decimals.
// The ambiguous "single/multiple dot, no comma" case is disambiguated by a
// heuristic: a lone dot followed by 1–2 digits is a decimal (5694.08, 1.5);
// dots followed by 3 digits, or multiple dots, are thousands (1.000, 1.000.000).
// Words (10 mil) are out of scope — only the LLM path normalizes those.
func parseARAmount(s string) (decimal.Decimal, error) {
	// keep only digits, dot, comma, minus (internal callers pass signed amounts)
	var b strings.Builder
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' || r == ',' || r == '-' {
			b.WriteRune(r)
		}
	}
	s = b.String()

	if strings.IndexByte(s, ',') >= 0 {
		// comma present: dots are thousands, comma is the decimal point
		s = strings.ReplaceAll(s, ".", "")
		s = strings.Replace(s, ",", ".", 1)
		return decimal.NewFromString(s)
	}

	// no comma: disambiguate the dot(s)
	if strings.Count(s, ".") > 1 {
		s = strings.ReplaceAll(s, ".", "") // multiple dots => thousands
	} else if i := strings.IndexByte(s, '.'); i >= 0 {
		if decimals := len(s) - i - 1; decimals == 3 {
			s = strings.ReplaceAll(s, ".", "") // single dot + 3 digits => thousands
		}
		// single dot + 1–2 digits => leave as decimal
	}
	return decimal.NewFromString(s)
}
