package currency

import "testing"

func TestLabel_NamesTheCurrencyInWords(t *testing.T) {
	for cur, want := range map[Currency]string{ARS: "pesos", USD: "dólares"} {
		if got := cur.Label(); got != want {
			t.Errorf("%s.Label() = %q, want %q", cur, got, want)
		}
		if cur.String() != string(cur) {
			t.Errorf("%s.String() dejó de ser el código ISO", cur)
		}
	}

	if got := Currency("EUR").Label(); got != "EUR" {
		t.Errorf("moneda sin traducir: got %q, want el código", got)
	}
}
