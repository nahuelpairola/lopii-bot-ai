package messaging

import (
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/orchestrator"
)

func TestEncodeDecodeMovementRows_RoundTrip(t *testing.T) {
	rows := []movementRow{
		{Type: "expense", Amount: "3000", Currency: "ARS", Category: "Alimentación", Subcategory: "Café"},
		{Type: "transfer", Amount: "100", Currency: "USD", AccountID: "5"},
	}

	data := conversation.Data{"movements": encodeMovementRows(rows)}
	got := decodeMovementRows(data)

	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}
	if got[0].Amount != "3000" || got[0].Category != "Alimentación" {
		t.Errorf("row 0 = %+v", got[0])
	}
	if got[1].AccountID != "5" {
		t.Errorf("row 1 account id = %q, want 5", got[1].AccountID)
	}
}

func TestEncodeDecodeStringSlice_RoundTrip(t *testing.T) {
	data := conversation.Data{"gaps": encodeStringSlice([]string{"0", "2"})}
	got := decodeStringSlice(data, "gaps")

	if len(got) != 2 || got[0] != "0" || got[1] != "2" {
		t.Errorf("got %v, want [0 2]", got)
	}
}

func TestDecodeStringSlice_MissingKey_ReturnsEmpty(t *testing.T) {
	got := decodeStringSlice(conversation.Data{}, "missing")
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestBuildCreateSeed_QueuesCategoryAndAccountGaps(t *testing.T) {
	result := orchestrator.CreateResult{
		Movements: []orchestrator.MovementDraft{
			{Type: "expense", Amount: "100000", Currency: "ARS", Category: "PENDING_REVIEW", Subcategory: "PENDING_REVIEW"},
			{Type: "transfer", Amount: "100", Currency: "USD", AccountNameGuess: "FCI", Category: "Inversiones", Subcategory: "FCI"},
		},
	}

	data := buildCreateSeed(result)

	categoryGaps := decodeStringSlice(data, "pending_category_gaps")
	if len(categoryGaps) != 1 || categoryGaps[0] != "0" {
		t.Errorf("category gaps = %v, want [0]", categoryGaps)
	}
	accountGaps := decodeStringSlice(data, "pending_account_gaps")
	if len(accountGaps) != 1 || accountGaps[0] != "1" {
		t.Errorf("account gaps = %v, want [1]", accountGaps)
	}
	if data["mode"] != "create" {
		t.Errorf("mode = %v, want create", data["mode"])
	}
}

func TestBuildCreateSeed_NoGaps_EmptyQueues(t *testing.T) {
	result := orchestrator.CreateResult{
		Movements: []orchestrator.MovementDraft{
			{Type: "expense", Amount: "3000", Currency: "ARS", Category: "Alimentación", Subcategory: "Café"},
		},
	}

	data := buildCreateSeed(result)

	if len(decodeStringSlice(data, "pending_category_gaps")) != 0 {
		t.Error("expected no category gaps")
	}
	if len(decodeStringSlice(data, "pending_account_gaps")) != 0 {
		t.Error("expected no account gaps")
	}
}

func TestParseUintSlice(t *testing.T) {
	got, err := parseUintSlice([]string{"1", "2", "30"})
	if err != nil {
		t.Fatalf("parseUintSlice: %v", err)
	}
	want := []uint{1, 2, 30}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestParseUintSlice_ErrorsOnGarbage(t *testing.T) {
	if _, err := parseUintSlice([]string{"not-a-number"}); err == nil {
		t.Fatal("expected an error for a non-numeric id")
	}
}
