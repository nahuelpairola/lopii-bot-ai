package messaging

import (
	"strings"
	"testing"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

func TestMsgConfirmUpdateDiff_ShowsAccountChange(t *testing.T) {
	before := []movementRow{{Amount: "610503", Currency: "ARS", AccountName: "Efectivo"}}
	after := []movementRow{{Category: "Deudas", Subcategory: "Tarjeta", Amount: "610503", Currency: "ARS", Description: "Pago tarjeta", Date: "2026-07-14", AccountName: "Galicia"}}
	data := conversation.Data{
		"before_movements": encodeMovementRows(before),
		"movements":        encodeMovementRows(after),
	}

	msg := msgConfirmUpdateDiff(data)
	for _, want := range []string{"Galicia", "Efectivo"} {
		if !strings.Contains(msg, want) {
			t.Errorf("diff %q missing %q — account change must be visible", msg, want)
		}
	}
}

func TestMovementReceiptLine_ShowsAccountWhenLoaded(t *testing.T) {
	m := movement.Movement{
		Type:        movement.Expense,
		Amount:      mustDecimal(t, "610503"),
		Currency:    "ARS",
		Subcategory: &subcategory.Subcategory{Category: "Deudas", Subcategory: "Tarjeta"},
		Date:        mustDate(t, "2026-07-14"),
		Account:     &account.Account{Name: "Efectivo"},
	}
	if line := movementReceiptLine(m); !strings.Contains(line, "Efectivo") {
		t.Errorf("receipt line %q should show the account name", line)
	}

	m.Account = nil // not loaded → must not panic, must not add a stray separator
	if line := movementReceiptLine(m); strings.Contains(line, "· ·") {
		t.Errorf("receipt line %q added an empty account segment", line)
	}
}

func TestMovementReceiptLine_IncludesCategorySubcategoryDescriptionDate(t *testing.T) {
	m := movement.Movement{
		Type:        movement.Expense,
		Amount:      mustDecimal(t, "3000"),
		Currency:    "ARS",
		Subcategory: &subcategory.Subcategory{Category: "Alimentación", Subcategory: "Café"},
		Description: strPtr("Café con Juan"),
		Date:        mustDate(t, "2026-07-04"),
	}

	line := movementReceiptLine(m)
	for _, want := range []string{"Alimentación", "Café", "Café con Juan", "2026-07-04", "3000", "ARS"} {
		if !strings.Contains(line, want) {
			t.Errorf("receipt line %q missing %q", line, want)
		}
	}
}

func TestMsgConfirmUpdateDiff_IncludesSubcategoryDescriptionDate(t *testing.T) {
	before := []movementRow{{Amount: "3000", Currency: "ARS"}}
	after := []movementRow{{Category: "Alimentación", Subcategory: "Café", Amount: "3500", Currency: "ARS", Description: "Café con Juan", Date: "2026-07-04"}}
	data := conversation.Data{
		"before_movements": encodeMovementRows(before),
		"movements":        encodeMovementRows(after),
	}

	msg := msgConfirmUpdateDiff(data)
	for _, want := range []string{"Alimentación", "Café", "Café con Juan", "2026-07-04"} {
		if !strings.Contains(msg, want) {
			t.Errorf("diff message %q missing %q", msg, want)
		}
	}
}

func TestMsgConfirmDelete_IncludesSubcategoryDescriptionDate(t *testing.T) {
	groups := []transactionGroup{{Movements: []movement.Movement{{
		SubcategoryID: 1,
		Subcategory:   &subcategory.Subcategory{Category: "Transporte", Subcategory: "Nafta"},
		Amount:        mustDecimal(t, "15000"),
		Currency:      "ARS",
		Description:   strPtr("Nafta YPF"),
		Date:          mustDate(t, "2026-07-04"),
	}}}}
	data := conversation.Data{
		"resolved_index":   "0",
		"candidate_groups": encodeCandidateGroups(groups),
	}

	msg := msgConfirmDelete(data)
	for _, want := range []string{"Transporte", "Nafta", "Nafta YPF", "2026-07-04"} {
		if !strings.Contains(msg, want) {
			t.Errorf("delete confirm message %q missing %q", msg, want)
		}
	}
}

func TestCapabilitiesShowcase_NoQueryExample(t *testing.T) {
	// QUERY isn't supported yet — no "¿cuánto gasté?"-style example may appear.
	for _, banned := range []string{"cuánto", "gasté esta", "cuanto"} {
		if strings.Contains(strings.ToLower(msgCapabilitiesShowcase), strings.ToLower(banned)) {
			t.Errorf("capabilities showcase must not contain a QUERY example (%q)", banned)
		}
	}
	// All six supported intents present.
	for _, want := range []string{"Anotar", "Corregir", "Borrar", "Transferir", "Nueva cuenta", "Nueva categoría"} {
		if !strings.Contains(msgCapabilitiesShowcase, want) {
			t.Errorf("capabilities showcase missing %q", want)
		}
	}
	// Natural number words, never "k".
	if strings.Contains(msgCapabilitiesShowcase, "10k") || strings.Contains(msgCapabilitiesShowcase, "50k") {
		t.Errorf("capabilities showcase uses \"k\" — must use natural words (mil/millón)")
	}
}

func TestOnboardingReceipt_ListsEachAccount(t *testing.T) {
	got := msgOnboardingReceipt([]onboardingRow{
		{Name: "Banco", Currency: "ARS", Balance: "20000"},
		{Name: "Bróker", Currency: "USD", Balance: "100"},
	})
	for _, want := range []string{"Banco", "20000", "ARS", "Bróker", "100", "USD"} {
		if !strings.Contains(got, want) {
			t.Errorf("receipt missing %q; got:\n%s", want, got)
		}
	}
}
