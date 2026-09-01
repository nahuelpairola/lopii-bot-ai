package messaging

import (
	"strings"
	"testing"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

func TestMsgConfirmUpdateDiff_ShowsAccountChange(t *testing.T) {
	before := []movement.MovementRow{{Amount: "610503", Currency: "ARS", AccountName: "Efectivo"}}
	after := []movement.MovementRow{{Category: "Deudas", Subcategory: "Tarjeta", Amount: "610503", Currency: "ARS", Description: "Pago tarjeta", Date: "2026-07-14", AccountName: "Galicia"}}
	data := conversation.Data{
		"before_movements": movement.EncodeMovementRows(before),
		"movements":        movement.EncodeMovementRows(after),
	}

	msg := flow.MsgConfirmUpdateDiff(data)
	for _, want := range []string{"Galicia", "Efectivo"} {
		if !strings.Contains(msg, want) {
			t.Errorf("diff %q missing %q — account change must be visible", msg, want)
		}
	}
}

func TestMovementGapDescriptor_UsesDescription(t *testing.T) {
	row := movement.MovementRow{Amount: "5000", Description: "compra en el super"}
	if got := movement.MovementGapDescriptor(row); got != "$5000 · compra en el super" {
		t.Errorf("want description, got %q", got)
	}
}

func TestAskPrompts_DistinguishRows(t *testing.T) {
	rows := []movement.MovementRow{
		{Amount: "5000", Description: "compra en Coto", Category: "PENDING_REVIEW"},
		{Amount: "50000", AccountNameGuess: "Mercado Pago", Description: "transferencia a Mercado Pago", Category: "PENDING_REVIEW"},
	}
	data := conversation.Data{
		conversation.KeyMovements:           movement.EncodeMovementRows(rows),
		conversation.KeyPendingCategoryGaps: conversation.EncodeStringSlice([]string{"0", "1"}),
	}

	prompt0 := flow.MsgAskCategory(data)
	if !strings.Contains(prompt0, "5000") || !strings.Contains(prompt0, "Coto") {
		t.Errorf("row 0 prompt missing its own data: %q", prompt0)
	}
	if !strings.Contains(prompt0, "1 de 2") {
		t.Errorf("row 0 prompt missing position counter: %q", prompt0)
	}

	data[conversation.KeyPendingCategoryGaps] = conversation.EncodeStringSlice([]string{"1"})
	prompt1 := flow.MsgAskCategory(data)
	if prompt1 == prompt0 {
		t.Fatalf("row 1 prompt identical to row 0's — this is the reported bug")
	}
	if !strings.Contains(prompt1, "50000") || !strings.Contains(prompt1, "Mercado Pago") {
		t.Errorf("row 1 prompt missing its own data: %q", prompt1)
	}
	if !strings.Contains(prompt1, "2 de 2") {
		t.Errorf("row 1 prompt missing position counter: %q", prompt1)
	}

	data["gap_active_row"] = "0"
	rows[0].Category = "Alimentación"
	data[conversation.KeyMovements] = movement.EncodeMovementRows(rows)
	subPrompt := flow.MsgAskSubcategory(data)
	if !strings.Contains(subPrompt, "Coto") || !strings.Contains(subPrompt, "Alimentación") {
		t.Errorf("subcategory prompt missing row+category context: %q", subPrompt)
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

	m.Account = nil
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
	for _, want := range []string{"Alimentación", "Café", "Café con Juan", "04/07", "$3.000"} {
		if !strings.Contains(line, want) {
			t.Errorf("receipt line %q missing %q", line, want)
		}
	}
	if strings.Contains(line, "ARS") {
		t.Errorf("receipt line %q repite la moneda al lado del símbolo", line)
	}
}

func TestMsgConfirmUpdateDiff_IncludesSubcategoryDescriptionDate(t *testing.T) {
	before := []movement.MovementRow{{Amount: "3000", Currency: "ARS"}}
	after := []movement.MovementRow{{Category: "Alimentación", Subcategory: "Café", Amount: "3500", Currency: "ARS", Description: "Café con Juan", Date: "2026-07-04"}}
	data := conversation.Data{
		"before_movements": movement.EncodeMovementRows(before),
		"movements":        movement.EncodeMovementRows(after),
	}

	msg := flow.MsgConfirmUpdateDiff(data)
	for _, want := range []string{"Alimentación", "Café", "Café con Juan", "04/07"} {
		if !strings.Contains(msg, want) {
			t.Errorf("diff message %q missing %q", msg, want)
		}
	}
	for _, want := range []string{"$3.500", "antes: $3.000"} {
		if !strings.Contains(msg, want) {
			t.Errorf("diff message %q missing %q", msg, want)
		}
	}
}
