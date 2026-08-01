package currency

import "testing"

// TestLabel_NamesTheCurrencyInWords: Label() es la fuente única de cómo se
// nombra una moneda en todo lo que lee el usuario (Telegram y Mini App). Si
// devolviera el código, "ARS" volvería a filtrarse a los ~15 sitios de copy de
// una sola vez.
func TestLabel_NamesTheCurrencyInWords(t *testing.T) {
	for cur, want := range map[Currency]string{ARS: "pesos", USD: "dólares"} {
		if got := cur.Label(); got != want {
			t.Errorf("%s.Label() = %q, want %q", cur, got, want)
		}
		// String() sigue siendo el código: es lo que va a la DB y a los Value de
		// los botones. Que las dos se separen es el punto de tener Label().
		if cur.String() != string(cur) {
			t.Errorf("%s.String() dejó de ser el código ISO", cur)
		}
	}

	// Una moneda sin traducir cae al código en vez de a vacío: un mensaje feo se
	// nota y se arregla; uno con un hueco pasa desapercibido.
	if got := Currency("EUR").Label(); got != "EUR" {
		t.Errorf("moneda sin traducir: got %q, want el código", got)
	}
}
