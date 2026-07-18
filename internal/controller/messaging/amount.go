package messaging

import (
	"strings"

	"github.com/shopspring/decimal"
)

// parseARAmount parses a money string in Argentine convention — comma is the
// decimal separator, dots are thousands — into a decimal. The LLM is told to
// pre-normalize amounts (orchestrator.numberFormatRule), but it is
// non-deterministic and user-typed amounts (account balance, balance adjust)
// never pass through it — so this deterministic parser is the guarantee that a
// comma amount is honored, whatever the source. Already-normalized dot strings
// pass straight through.
func parseARAmount(s string) (decimal.Decimal, error) {
	s = strings.TrimSpace(s)
	if strings.IndexByte(s, ',') >= 0 {
		s = strings.ReplaceAll(s, ".", "")  // dots are thousands separators
		s = strings.Replace(s, ",", ".", 1) // the comma is the decimal point
	}
	return decimal.NewFromString(s)
}
