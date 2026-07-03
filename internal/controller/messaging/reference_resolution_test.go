package messaging

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
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

// fakeMovementRepoForResolve implements movementRepository just to drive
// resolveCandidates in isolation: it records the (query, since) it was
// called with and returns a canned result set.
type fakeMovementRepoForResolve struct {
	result        []movement.Movement
	err           error
	capturedQuery string
	capturedSince time.Time
}

func (r *fakeMovementRepoForResolve) InsertBatch(ms []movement.Movement) error { return nil }
func (r *fakeMovementRepoForResolve) SumAmountForAccount(accountID uint64) (decimal.Decimal, error) {
	return decimal.Zero, nil
}
func (r *fakeMovementRepoForResolve) ReplaceMovements(oldIDs []uint, newMovements []movement.Movement) error {
	return nil
}
func (r *fakeMovementRepoForResolve) FindSimilarForUser(userID uint64, query string, since time.Time) ([]movement.Movement, error) {
	r.capturedQuery = query
	r.capturedSince = since
	return r.result, r.err
}
func (r *fakeMovementRepoForResolve) SoftDeleteByIDs(ids []uint) error { return nil }

func TestResolveCandidates_NoMentionedDate_UsesSevenDayCap(t *testing.T) {
	fake := &fakeMovementRepoForResolve{result: []movement.Movement{{Description: strPtr("Nafta YPF")}}}
	c := &controller{movements: fake}

	candidates, err := c.resolveCandidates(42, "che, lo de la nafta ypf era otro monto", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("got %d candidates, want 1", len(candidates))
	}

	wantSince := time.Now().Add(-7 * 24 * time.Hour)
	delta := fake.capturedSince.Sub(wantSince)
	if delta < -5*time.Second || delta > 5*time.Second {
		t.Errorf("since = %v, want close to %v (delta %v)", fake.capturedSince, wantSince, delta)
	}
}

func TestResolveCandidates_MentionedDate_AnchorsNoCap(t *testing.T) {
	fake := &fakeMovementRepoForResolve{result: []movement.Movement{{Description: strPtr("Nafta YPF")}}}
	c := &controller{movements: fake}

	mentionedDate := "2026-06-01" // well outside the default 7-day window
	candidates, err := c.resolveCandidates(42, "che, lo de la nafta ypf era otro monto", mentionedDate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("got %d candidates, want 1", len(candidates))
	}

	anchor, err := time.Parse("2006-01-02", mentionedDate)
	if err != nil {
		t.Fatalf("parse anchor date: %v", err)
	}
	wantSince := anchor.Add(-24 * time.Hour)
	if !fake.capturedSince.Equal(wantSince) {
		t.Errorf("since = %v, want %v (anchored on mentioned date, margin applied)", fake.capturedSince, wantSince)
	}

	sevenDaysAgo := time.Now().Add(-7 * 24 * time.Hour)
	if !fake.capturedSince.Before(sevenDaysAgo) {
		t.Errorf("since = %v is not before the 7-day cap (%v) — the cap was not lifted", fake.capturedSince, sevenDaysAgo)
	}
}
