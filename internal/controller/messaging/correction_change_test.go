package messaging

import (
	"errors"
	"testing"

	"lopiibot.com/internal/movement"
)

func TestApplyChange_Amount(t *testing.T) {
	cases := []struct {
		name    string
		start   string
		ch      correctionChange
		want    string
		wantErr error
	}{
		{"set reemplaza", "1000", correctionChange{fieldAmount, opSet, "2000"}, "2000", nil},
		{"add suma", "1000", correctionChange{fieldAmount, opAdd, "70"}, "1070", nil},
		{"subtract resta", "1000", correctionChange{fieldAmount, opSubtract, "100"}, "900", nil},
		{"multiply la mitad", "1000", correctionChange{fieldAmount, opMultiply, "0.5"}, "500", nil},
		{"multiply el doble", "1000", correctionChange{fieldAmount, opMultiply, "2"}, "2000", nil},

		// Bordes que destaparon reglas que la spec no tenía escritas.
		{"multiply redondea a 2 decimales", "1001", correctionChange{fieldAmount, opMultiply, "0.5"}, "500.5", nil},
		{"multiply del 30% redondea", "1001", correctionChange{fieldAmount, opMultiply, "0.3"}, "300.3", nil},
		{"formato argentino en el valor", "1000", correctionChange{fieldAmount, opSet, "1.070,50"}, "1070.5", nil},

		// El reintegro mayor que la compra daría vuelta el signo, y Normalize lo
		// re-firmaría como INGRESO. Un gasto no se convierte en ingreso en silencio.
		{"reintegro mayor que la compra", "700", correctionChange{fieldAmount, opSubtract, "2000"}, "", errRefundExceedsAmount},
		{"reintegro exacto llega a cero", "700", correctionChange{fieldAmount, opSubtract, "700"}, "0", nil},

		// Nunca coercionar a cero: así es como un "no entendí" se vuelve un borrado.
		{"valor ilegible", "1000", correctionChange{fieldAmount, opSet, "asd"}, "", errUnparseableValue},
		{"valor con abreviatura", "1000", correctionChange{fieldAmount, opSet, "2k"}, "", errUnparseableValue},
		{"monto de origen ilegible", "", correctionChange{fieldAmount, opAdd, "100"}, "", errUnparseableValue},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := applyChange(movement.MovementRow{Type: "expense", Amount: tc.start, Currency: "ARS"}, tc.ch)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil && got.Amount != tc.want {
				t.Errorf("amount = %q, want %q", got.Amount, tc.want)
			}
		})
	}
}

// Sólo amount acepta aritmética. Sin esta guarda un `add` sobre currency o date
// compila, corre y hace cualquier cosa.
func TestApplyChange_ArithmeticOnlyOnAmount(t *testing.T) {
	row := movement.MovementRow{Amount: "1000", Description: "Café"}
	for _, f := range []changeField{fieldCategory, fieldAccount, fieldDate, fieldCurrency, fieldDescription, fieldType} {
		for _, op := range []changeOp{opAdd, opSubtract, opMultiply} {
			if _, err := applyChange(row, correctionChange{f, op, "1"}); !errors.Is(err, errOpNotForField) {
				t.Errorf("applyChange(%s, %s) = %v, want errOpNotForField", f, op, err)
			}
		}
	}
}

func TestApplyChange_SetFields(t *testing.T) {
	base := movement.MovementRow{
		Type: "expense", Amount: "1000", Currency: "ARS",
		Category: "Alimentación", Subcategory: "Supermercado",
		AccountID: "7", AccountNameGuess: "", Date: "2026-08-01", Description: "super",
	}

	t.Run("category limpia la subcategoría", func(t *testing.T) {
		// El usuario dice "proyecto hogar/agua" como UNA cosa: el par lo resuelve
		// el gap contra la taxonomía, no el modelo.
		got, err := applyChange(base, correctionChange{fieldCategory, opSet, "proyecto hogar"})
		if err != nil {
			t.Fatal(err)
		}
		if got.Category != "proyecto hogar" || got.Subcategory != "" {
			t.Errorf("category/subcategory = %q/%q", got.Category, got.Subcategory)
		}
	})

	t.Run("account limpia el id resuelto", func(t *testing.T) {
		// Sin esto el AccountID viejo le ganaría al nombre nuevo y la corrección
		// no movería la plata de cuenta: un bug de saldos, no cosmético.
		got, err := applyChange(base, correctionChange{fieldAccount, opSet, "Galicia"})
		if err != nil {
			t.Fatal(err)
		}
		if got.AccountNameGuess != "Galicia" || got.AccountID != "" {
			t.Errorf("account = %q, id = %q (el id viejo tiene que limpiarse)", got.AccountNameGuess, got.AccountID)
		}
	})

	t.Run("currency", func(t *testing.T) {
		got, _ := applyChange(base, correctionChange{fieldCurrency, opSet, "USD"})
		if got.Currency != "USD" {
			t.Errorf("currency = %q", got.Currency)
		}
	})

	t.Run("type expense a income", func(t *testing.T) {
		got, err := applyChange(base, correctionChange{fieldType, opSet, "income"})
		if err != nil {
			t.Fatal(err)
		}
		if got.Type != "income" {
			t.Errorf("type = %q", got.Type)
		}
	})
}

// transfer necesita dos patas y una contraparte: convertir un gasto en
// transferencia es un delete + create, no una corrección.
func TestApplyChange_TypeRejectsTransfer(t *testing.T) {
	row := movement.MovementRow{Type: "expense", Amount: "1000"}
	if _, err := applyChange(row, correctionChange{fieldType, opSet, "transfer"}); !errors.Is(err, errTransferNotACorrection) {
		t.Errorf("err = %v, want errTransferNotACorrection", err)
	}
}

func TestApplyChange_DoesNotMutateInput(t *testing.T) {
	row := movement.MovementRow{Type: "expense", Amount: "1000", Currency: "ARS"}
	if _, err := applyChange(row, correctionChange{fieldAmount, opSet, "9999"}); err != nil {
		t.Fatal(err)
	}
	if row.Amount != "1000" {
		t.Errorf("la fila de entrada se mutó: %q", row.Amount)
	}
}

// Regresión del 400 del 2026-08-12. El modelo omite `op` cuando no significa
// nada —o sea en los seis campos que no son amount— y antes eso era un campo
// REQUERIDO del schema: Groq validaba del lado del servidor y devolvía un 400
// duro, matando el turno entero como "unclear".
//
// Sin defaultChangeOps, applyChange rechaza el cambio con errOpNotForField,
// porque un op vacío no es opSet.
func TestDefaultChangeOps_FillsTheOmittedSet(t *testing.T) {
	// Tal cual lo emitió el modelo ante "El café de hoy fue en un bar".
	changes := []correctionChange{{Field: fieldDescription, Value: "bar"}}
	defaultChangeOps(changes)

	if changes[0].Op != opSet {
		t.Fatalf("op = %q, want %q", changes[0].Op, opSet)
	}
	got, err := applyChange(movement.MovementRow{Type: "expense", Amount: "3500", Description: "Café"}, changes[0])
	if err != nil {
		t.Fatalf("applyChange: %v", err)
	}
	if got.Description != "bar" {
		t.Errorf("description = %q, want %q", got.Description, "bar")
	}
}

// El op explícito NO se pisa: si el modelo dice "sumale", tiene que seguir
// siendo una suma.
func TestDefaultChangeOps_KeepsAnExplicitOp(t *testing.T) {
	changes := []correctionChange{{Field: fieldAmount, Op: opAdd, Value: "1070"}}
	defaultChangeOps(changes)

	if changes[0].Op != opAdd {
		t.Fatalf("op = %q, want %q — un op explícito no se pisa", changes[0].Op, opAdd)
	}
}
