package orchestrator

import "testing"

// Nivel 0: la parte del clasificador que NO necesita modelo. Corre en la suite
// por default, sin key y sin red.
func TestStructuralPair(t *testing.T) {
	cases := []struct {
		name    string
		rows    []MovementDraft
		wantCat string
		wantSub string
		wantOK  bool
	}{
		{"2 patas, una moneda, balancean",
			[]MovementDraft{{Amount: "5000", Currency: "ARS"}, {Amount: "5000", Currency: "ARS"}},
			catSistema, subTransferencia, true},
		{"2 patas, dos monedas",
			[]MovementDraft{{Amount: "100", Currency: "USD"}, {Amount: "120000", Currency: "ARS"}},
			catInversiones, subDolares, true},

		// Los negativos importan tanto como los positivos: una regla que dispara
		// de más le saca al clasificador un caso que sí sabe resolver.
		{"2 patas que NO balancean",
			[]MovementDraft{{Amount: "5000", Currency: "ARS"}, {Amount: "4000", Currency: "ARS"}},
			"", "", false},
		{"una sola pata", []MovementDraft{{Amount: "5000", Currency: "ARS"}}, "", "", false},
		{"tres patas",
			[]MovementDraft{{Amount: "1", Currency: "ARS"}, {Amount: "1", Currency: "ARS"}, {Amount: "2", Currency: "ARS"}},
			"", "", false},
		{"sin filas", nil, "", "", false},
		{"monto ilegible", []MovementDraft{{Amount: "asd", Currency: "ARS"}, {Amount: "5000", Currency: "ARS"}}, "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cat, sub, ok := structuralPair(tc.rows)
			if ok != tc.wantOK || cat != tc.wantCat || sub != tc.wantSub {
				t.Errorf("structuralPair = (%q,%q,%v), want (%q,%q,%v)", cat, sub, ok, tc.wantCat, tc.wantSub, tc.wantOK)
			}
		})
	}
}

// La razón por la que el par estructural es un DEFAULT y no una regla dura:
// una suscripción de FCI tiene EXACTAMENTE la misma forma que una
// transferencia, y `Inversiones | FCI` existe como par propio en la taxonomía.
// Sólo el mensaje las distingue, así que el clasificador tiene que poder pisar
// esto.
func TestStructuralPair_FCILooksExactlyLikeATransfer(t *testing.T) {
	fci := []MovementDraft{
		{Amount: "50000", Currency: "ARS"},
		{Amount: "50000", Currency: "ARS"},
	}
	cat, sub, ok := structuralPair(fci)
	if !ok || cat != catSistema || sub != subTransferencia {
		t.Fatalf("structuralPair = (%q,%q,%v): la forma es indistinguible de una transferencia", cat, sub, ok)
	}
	// Si algún día esto se vuelve una regla DURA, este test tiene que romperse:
	// significaría que ninguna suscripción de FCI puede clasificarse bien.
}
