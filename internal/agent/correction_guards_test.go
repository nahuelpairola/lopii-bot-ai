package agent

import (
	"errors"
	"testing"

	"lopiibot.com/internal/movement"
)

func transferPair() []movement.MovementRow {
	return []movement.MovementRow{
		{Type: "transfer", Amount: "5000", AccountID: "1", Currency: "ARS"},
		{Type: "transfer", Amount: "5000", AccountID: "2", Currency: "ARS"},
	}
}

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

func TestApplyChanges_SingleMovementAcceptsAccountChange(t *testing.T) {
	rows := []movement.MovementRow{{Type: "expense", Amount: "1000", AccountID: "1", Currency: "ARS"}}
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

func TestApplyChangesToSet_SetAmountOnManyIsRefused(t *testing.T) {
	groups := [][]movement.MovementRow{
		{{Type: "expense", Amount: "1000", Currency: "ARS"}},
		{{Type: "expense", Amount: "2000", Currency: "ARS"}},
	}
	_, err := applyChangesToSet(groups, []correctionChange{{fieldAmount, opSet, "1500"}}, guardContext{Scope: scopeAll})
	if !errors.Is(err, errAmbiguousSetAll) {
		t.Errorf("err = %v, want errAmbiguousSetAll", err)
	}
}

func TestApplyChangesToSet_MultiplyOnManyIsFine(t *testing.T) {
	groups := [][]movement.MovementRow{
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

func TestApplyChangesToSet_RefundIntoAnotherAccountIsRefused(t *testing.T) {
	groups := [][]movement.MovementRow{{{Type: "expense", Amount: "1000", AccountName: "Galicia", Currency: "ARS"}}}

	_, err := applyChangesToSet(groups, []correctionChange{{fieldAmount, opSubtract, "1000"}},
		guardContext{NamedAccount: "Mercado Pago", Scope: scopeOne})
	if !errors.Is(err, errRefundToOtherAccount) {
		t.Errorf("err = %v, want errRefundToOtherAccount", err)
	}
}

func TestApplyChangesToSet_RefundIntoTheSameAccountIsACorrection(t *testing.T) {
	groups := [][]movement.MovementRow{{{Type: "expense", Amount: "1000", AccountName: "Galicia", Currency: "ARS"}}}

	got, err := applyChangesToSet(groups, []correctionChange{{fieldAmount, opSubtract, "300"}},
		guardContext{NamedAccount: "Galicia", Scope: scopeOne})
	if err != nil {
		t.Fatal(err)
	}
	if got[0][0].Amount != "700" {
		t.Errorf("amount = %q, want 700", got[0][0].Amount)
	}
}

func TestApplyChangesToSet_RefundWithNoAccountNamedIsACorrection(t *testing.T) {
	groups := [][]movement.MovementRow{{{Type: "expense", Amount: "1000", AccountName: "Galicia", Currency: "ARS"}}}

	if _, err := applyChangesToSet(groups, []correctionChange{{fieldAmount, opSubtract, "300"}},
		guardContext{Scope: scopeOne}); err != nil {
		t.Fatalf("sin cuenta nombrada tiene que ser una corrección: %v", err)
	}
}

func TestApplyChangesToSet_RefundGuardOnlyAppliesToRefunds(t *testing.T) {
	groups := [][]movement.MovementRow{{{Type: "expense", Amount: "1000", AccountName: "Galicia", Currency: "ARS"}}}

	if _, err := applyChangesToSet(groups, []correctionChange{{fieldCategory, opSet, "Ocio"}},
		guardContext{NamedAccount: "Mercado Pago", Scope: scopeOne}); err != nil {
		t.Errorf("un cambio de categoría no es un reintegro: %v", err)
	}
}

func TestGuardRefundDirection_RejectsAnAddOnARefund(t *testing.T) {
	rows := []movement.MovementRow{{Type: "expense", Amount: "15000", Currency: "ARS", Description: "Nafta"}}
	changes := []correctionChange{{fieldAmount, opAdd, "7500"}}

	err := guardRefundDirection(rows, changes, "De la nafta me devolvieron la mitad")
	if !errors.Is(err, errRefundThatGrows) {
		t.Errorf("err = %v, want errRefundThatGrows", err)
	}
}

func TestGuardRefundDirection_RejectsAnyChangeThatGrows(t *testing.T) {
	rows := []movement.MovementRow{{Type: "expense", Amount: "15000", Currency: "ARS"}}
	for _, ch := range []correctionChange{
		{fieldAmount, opMultiply, "1.5"},
		{fieldAmount, opSet, "99999"},
	} {
		if err := guardRefundDirection(rows, []correctionChange{ch}, "me reintegraron algo"); !errors.Is(err, errRefundThatGrows) {
			t.Errorf("%+v: err = %v, want errRefundThatGrows", ch, err)
		}
	}
}

func TestGuardRefundDirection_AllowsAChangeThatShrinks(t *testing.T) {
	rows := []movement.MovementRow{{Type: "expense", Amount: "15000", Currency: "ARS"}}
	for _, ch := range []correctionChange{
		{fieldAmount, opMultiply, "0.5"},
		{fieldAmount, opSubtract, "7500"},
	} {
		if err := guardRefundDirection(rows, []correctionChange{ch}, "me devolvieron la mitad"); err != nil {
			t.Errorf("%+v: %v — un reintegro que reduce es lo correcto", ch, err)
		}
	}
}

func TestGuardRefundDirection_IgnoresMessagesWithoutARefund(t *testing.T) {
	rows := []movement.MovementRow{{Type: "expense", Amount: "12700", Currency: "ARS"}}
	changes := []correctionChange{{fieldAmount, opAdd, "1070"}}

	if err := guardRefundDirection(rows, changes, "al café de hoy sumale 1070"); err != nil {
		t.Errorf("err = %v: sin devolución, sumar es válido", err)
	}
}
