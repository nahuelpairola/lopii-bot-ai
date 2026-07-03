package messaging

import (
	"testing"

	"github.com/google/uuid"
	"lopiibot.com/internal/movement"
)

func strPtr(s string) *string { return &s }

func mustParseUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	u, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("parse uuid: %v", err)
	}
	return u
}

func TestGroupByTransaction_GroupsSharedID(t *testing.T) {
	txID := mustParseUUID(t, "11111111-1111-1111-1111-111111111111")
	ms := []movement.Movement{
		{TransactionID: &txID, Description: strPtr("Compra USD ARS leg")},
		{TransactionID: &txID, Description: strPtr("Compra USD USD leg")},
		{Description: strPtr("Standalone café")},
	}

	groups := groupByTransaction(ms)
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2 (one 2-row group, one standalone)", len(groups))
	}
	if len(groups[0].Movements) != 2 {
		t.Errorf("first group has %d movements, want 2", len(groups[0].Movements))
	}
	if len(groups[1].Movements) != 1 {
		t.Errorf("second group has %d movements, want 1", len(groups[1].Movements))
	}
}

func TestMatchesMessage_DescriptionSubstring(t *testing.T) {
	group := transactionGroup{Movements: []movement.Movement{{Description: strPtr("Nafta YPF")}}}
	if !matchesMessage(group, "che, lo de la nafta ypf era otro monto") {
		t.Error("expected a match on description substring")
	}
}

func TestMatchesMessage_NoOverlap(t *testing.T) {
	group := transactionGroup{Movements: []movement.Movement{{Description: strPtr("Sueldo")}}}
	if matchesMessage(group, "el café de ayer era 3000") {
		t.Error("expected no match")
	}
}
