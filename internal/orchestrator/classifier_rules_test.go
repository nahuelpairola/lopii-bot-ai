package orchestrator

import "testing"

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
			cat, sub, ok := StructuralPair(tc.rows)
			if ok != tc.wantOK || cat != tc.wantCat || sub != tc.wantSub {
				t.Errorf("structuralPair = (%q,%q,%v), want (%q,%q,%v)", cat, sub, ok, tc.wantCat, tc.wantSub, tc.wantOK)
			}
		})
	}
}

func TestStructuralPair_FCILooksExactlyLikeATransfer(t *testing.T) {
	fci := []MovementDraft{
		{Amount: "50000", Currency: "ARS"},
		{Amount: "50000", Currency: "ARS"},
	}
	cat, sub, ok := StructuralPair(fci)
	if !ok || cat != catSistema || sub != subTransferencia {
		t.Fatalf("structuralPair = (%q,%q,%v): la forma es indistinguible de una transferencia", cat, sub, ok)
	}
}
