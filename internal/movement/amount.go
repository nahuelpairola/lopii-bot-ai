package movement

import (
	"errors"
	"strings"
	"unicode"

	"github.com/shopspring/decimal"
)

var ErrAmountHasLetters = errors.New("amount contains letters")

func ParseARAmount(s string) (decimal.Decimal, error) {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return decimal.Zero, ErrAmountHasLetters
		}
	}

	var b strings.Builder
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' || r == ',' || r == '-' {
			b.WriteRune(r)
		}
	}
	s = b.String()

	if strings.IndexByte(s, ',') >= 0 {
		s = strings.ReplaceAll(s, ".", "")
		s = strings.Replace(s, ",", ".", 1)
		return decimal.NewFromString(s)
	}

	if strings.Count(s, ".") > 1 {
		s = strings.ReplaceAll(s, ".", "")
	} else if i := strings.IndexByte(s, '.'); i >= 0 {
		if decimals := len(s) - i - 1; decimals == 3 {
			s = strings.ReplaceAll(s, ".", "")
		}
	}
	return decimal.NewFromString(s)
}
