package templates

import (
	"testing"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/currency"
)

func TestFormatMoney(t *testing.T) {
	cases := []struct {
		name string
		in   decimal.Decimal
		cur  currency.Currency
		want string
	}{
		{"ARS redondea los centavos", decimal.RequireFromString("1234567.89"), currency.ARS, "$1.234.568"},
		{"ARS negativo lleva el signo afuera", decimal.RequireFromString("-560000"), currency.ARS, "-$560.000"},
		{"ARS cero", decimal.Zero, currency.ARS, "$0"},
		{"ARS sin miles", decimal.NewFromInt(999), currency.ARS, "$999"},
		{"USD conserva centavos con coma", decimal.RequireFromString("1234.56"), currency.USD, "US$1.234,56"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FormatMoney(c.in, c.cur); got != c.want {
				t.Errorf("FormatMoney = %q, want %q", got, c.want)
			}
		})
	}
}

func TestFormatCompact(t *testing.T) {
	cases := []struct {
		in   decimal.Decimal
		want string
	}{
		{decimal.Zero, "·"},
		{decimal.NewFromInt(840), "840"},
		{decimal.NewFromInt(412000), "412k"},
		{decimal.NewFromInt(999999), "1,0M"}, // no "1000k"
		{decimal.NewFromInt(1250000), "1,3M"},
	}
	for _, c := range cases {
		if got := FormatCompact(c.in); got != c.want {
			t.Errorf("FormatCompact(%s) = %q, want %q", c.in, got, c.want)
		}
	}
}
