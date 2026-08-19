package messaging

import (
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
)

func TestEncodeDecodeMovementRows_RoundTrip(t *testing.T) {
	rows := []movement.MovementRow{
		{Type: "expense", Amount: "3000", Currency: "ARS", Category: "Alimentación", Subcategory: "Café"},
		{Type: "transfer", Amount: "100", Currency: "USD", AccountID: "5"},
	}

	data := conversation.Data{"movements": movement.EncodeMovementRows(rows)}
	got := movement.DecodeMovementRows(data)

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
	data := conversation.Data{"gaps": conversation.EncodeStringSlice([]string{"0", "2"})}
	got := conversation.DecodeStringSlice(data, "gaps")

	if len(got) != 2 || got[0] != "0" || got[1] != "2" {
		t.Errorf("got %v, want [0 2]", got)
	}
}

func TestDecodeStringSlice_MissingKey_ReturnsEmpty(t *testing.T) {
	got := conversation.DecodeStringSlice(conversation.Data{}, "missing")
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestMovementRow_GroupRoundTrip(t *testing.T) {
	rows := []movement.MovementRow{{Type: "transfer", Amount: "100", Currency: "ARS", Group: "g1"}}
	data := conversation.Data{"movements": movement.EncodeMovementRows(rows)}
	got := movement.DecodeMovementRows(data)
	if len(got) != 1 || got[0].Group != "g1" {
		t.Fatalf("group round trip = %+v, want Group=g1", got)
	}
}

// TestCopyDataNeverReturnsNil — mismo invariante que conversation.cloneData:
// los ~36 call sites escriben sobre la copia, así que devolver nil paniquea.
func TestCopyDataNeverReturnsNil(t *testing.T) {
	got := conversation.CopyData(nil)
	if got == nil {
		t.Fatal("conversation.CopyData(nil) devolvió nil: el próximo write va a paniquear")
	}
	got[conversation.KeyCancelled] = "true" // no debe paniquear
}
