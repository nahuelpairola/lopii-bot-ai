package messaging

import (
	"errors"
	"testing"
)

func transferPair() []movementRow {
	return []movementRow{
		{Type: "transfer", Amount: "5000", AccountID: "1", Currency: "ARS"},
		{Type: "transfer", Amount: "5000", AccountID: "2", Currency: "ARS"},
	}
}

// Un cambio de monto va a las DOS patas. Si fuera a una sola, el grupo deja de
// balancear y el guard lo rechaza al insertar: la corrección fallaría ruidosa
// pero inútil.
func TestApplyChanges_TransferAmountGoesToBothLegs(t *testing.T) {
	got, err := applyChanges(transferPair(), []correctionChange{{fieldAmount, opSet, "8000"}})
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range got {
		if r.Amount != "8000" {
			t.Errorf("pata %d = %s, want 8000 en ambas", i, r.Amount)
		}
	}
}

func TestApplyChanges_TransferRejectsAccountChange(t *testing.T) {
	_, err := applyChanges(transferPair(), []correctionChange{{fieldAccount, opSet, "Galicia"}})
	if !errors.Is(err, errAccountOnTransfer) {
		t.Errorf("err = %v, want errAccountOnTransfer: 'cuál pata' no es expresable", err)
	}
}

// Un movimiento suelto SÍ puede cambiar de cuenta: la restricción es del grupo.
func TestApplyChanges_SingleMovementAcceptsAccountChange(t *testing.T) {
	rows := []movementRow{{Type: "expense", Amount: "1000", AccountID: "1", Currency: "ARS"}}
	got, err := applyChanges(rows, []correctionChange{{fieldAccount, opSet, "Galicia"}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].AccountNameGuess != "Galicia" {
		t.Errorf("account = %q", got[0].AccountNameGuess)
	}
}

func TestApplyChanges_DoesNotMutateInput(t *testing.T) {
	rows := transferPair()
	if _, err := applyChanges(rows, []correctionChange{{fieldAmount, opSet, "9999"}}); err != nil {
		t.Fatal(err)
	}
	if rows[0].Amount != "5000" {
		t.Errorf("se mutó la entrada: %q", rows[0].Amount)
	}
}

// "poné todos en 1500" no es algo que un usuario quiera decir: es un scope mal
// completado. Preguntar cuál es mejor que aplanar n montos al mismo número.
func TestApplyChangesToSet_SetAmountOnManyIsRefused(t *testing.T) {
	groups := [][]movementRow{
		{{Type: "expense", Amount: "1000", Currency: "ARS"}},
		{{Type: "expense", Amount: "2000", Currency: "ARS"}},
	}
	_, err := applyChangesToSet(groups, []correctionChange{{fieldAmount, opSet, "1500"}}, guardContext{Scope: scopeAll})
	if !errors.Is(err, errAmbiguousSetAll) {
		t.Errorf("err = %v, want errAmbiguousSetAll", err)
	}
}

// El mismo set con multiply SÍ pasa: cada fila queda en un valor distinto, que
// es justo el punto de hacer la cuenta del lado de la app.
func TestApplyChangesToSet_MultiplyOnManyIsFine(t *testing.T) {
	groups := [][]movementRow{
		{{Type: "expense", Amount: "1000", Currency: "ARS"}},
		{{Type: "expense", Amount: "2400", Currency: "ARS"}},
		{{Type: "expense", Amount: "80000", Currency: "ARS"}},
	}
	got, err := applyChangesToSet(groups, []correctionChange{{fieldAmount, opMultiply, "0.5"}}, guardContext{Scope: scopeAll})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"500", "1200", "40000"}
	for i, g := range got {
		if g[0].Amount != want[i] {
			t.Errorf("grupo %d = %s, want %s", i, g[0].Amount, want[i])
		}
	}
}

// El agujero contable: un reintegro que entró en OTRA cuenta es un ingreso, no
// una corrección. Restarlo del gasto original sube el saldo de la cuenta que
// pagó y deja la otra intacta — dos saldos mal.
func TestApplyChangesToSet_RefundIntoAnotherAccountIsRefused(t *testing.T) {
	groups := [][]movementRow{{{Type: "expense", Amount: "1000", AccountName: "Galicia", Currency: "ARS"}}}

	_, err := applyChangesToSet(groups, []correctionChange{{fieldAmount, opSubtract, "1000"}},
		guardContext{NamedAccount: "Mercado Pago", Scope: scopeOne})
	if !errors.Is(err, errRefundToOtherAccount) {
		t.Errorf("err = %v, want errRefundToOtherAccount", err)
	}
}

func TestApplyChangesToSet_RefundIntoTheSameAccountIsACorrection(t *testing.T) {
	groups := [][]movementRow{{{Type: "expense", Amount: "1000", AccountName: "Galicia", Currency: "ARS"}}}

	got, err := applyChangesToSet(groups, []correctionChange{{fieldAmount, opSubtract, "300"}},
		guardContext{NamedAccount: "Galicia", Scope: scopeOne})
	if err != nil {
		t.Fatal(err)
	}
	if got[0][0].Amount != "700" {
		t.Errorf("amount = %q, want 700", got[0][0].Amount)
	}
}

// El default abrumador: el usuario no dice a dónde volvió la plata.
func TestApplyChangesToSet_RefundWithNoAccountNamedIsACorrection(t *testing.T) {
	groups := [][]movementRow{{{Type: "expense", Amount: "1000", AccountName: "Galicia", Currency: "ARS"}}}

	if _, err := applyChangesToSet(groups, []correctionChange{{fieldAmount, opSubtract, "300"}},
		guardContext{Scope: scopeOne}); err != nil {
		t.Fatalf("sin cuenta nombrada tiene que ser una corrección: %v", err)
	}
}

// La guarda del reintegro mira SOLO subtract/multiply: un set de monto o un
// cambio de categoría que mencione una cuenta no tiene por qué rechazarse.
func TestApplyChangesToSet_RefundGuardOnlyAppliesToRefunds(t *testing.T) {
	groups := [][]movementRow{{{Type: "expense", Amount: "1000", AccountName: "Galicia", Currency: "ARS"}}}

	if _, err := applyChangesToSet(groups, []correctionChange{{fieldCategory, opSet, "Ocio"}},
		guardContext{NamedAccount: "Mercado Pago", Scope: scopeOne}); err != nil {
		t.Errorf("un cambio de categoría no es un reintegro: %v", err)
	}
}
