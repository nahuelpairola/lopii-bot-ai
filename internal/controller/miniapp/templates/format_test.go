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
		name string
		in   decimal.Decimal
		cur  currency.Currency
		want string
	}{
		{"ARS cero es el punto muted", decimal.Zero, currency.ARS, "·"},
		{"ARS no-cero que redondea a cero avisa que hubo gasto", decimal.NewFromInt(450), currency.ARS, "<1"},
		{"ARS en el piso exacto ya redondea a 1", decimal.NewFromInt(500), currency.ARS, "1"},
		{"ARS redondea hacia arriba", decimal.NewFromInt(840), currency.ARS, "1"},
		{"ARS caso típico", decimal.NewFromInt(560000), currency.ARS, "560"},
		{"ARS agrupa miles dentro de la escala de miles", decimal.NewFromInt(1240000), currency.ARS, "1.240"},
		{"ARS negativo muestra la magnitud", decimal.NewFromInt(-560000), currency.ARS, "560"},
		{"USD cero es el mismo punto muted", decimal.Zero, currency.USD, "·"},
		{"USD no se escala", decimal.NewFromInt(120), currency.USD, "120"},
		{"USD agrupa pero no escala", decimal.NewFromInt(1200), currency.USD, "1.200"},
		{"USD redondea los centavos", decimal.RequireFromString("1234.56"), currency.USD, "1.235"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FormatCompact(c.in, c.cur); got != c.want {
				t.Errorf("FormatCompact(%s, %s) = %q, want %q", c.in, c.cur, got, c.want)
			}
		})
	}
}

func TestScaleNote(t *testing.T) {
	if got := ScaleNote(currency.ARS); got != " · en miles de $" {
		t.Errorf("ScaleNote(ARS) = %q, want %q", got, " · en miles de $")
	}
	if got := ScaleNote(currency.USD); got != "" {
		t.Errorf("ScaleNote(USD) = %q, want empty", got)
	}
}
