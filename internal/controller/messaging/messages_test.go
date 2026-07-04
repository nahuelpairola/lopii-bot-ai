package messaging

import (
	"strings"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

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
