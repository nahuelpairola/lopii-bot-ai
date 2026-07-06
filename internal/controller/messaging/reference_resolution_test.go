package messaging

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

func TestMatchesMessage_MultiWordDescription_SharedToken(t *testing.T) {
	// "gasto en trabas" stored; user says only "...de trabas" — one shared word.
	group := transactionGroup{Movements: []movement.Movement{{Description: strPtr("gasto en trabas")}}}
	if !matchesMessage(group, "quiero eliminar mi registro de trabas") {
		t.Error("expected a match on the shared token 'trabas'")
	}
}

func TestMatchesMessage_ShortWordInLongMessage(t *testing.T) {
	group := transactionGroup{Movements: []movement.Movement{{Description: strPtr("Café")}}}
	if !matchesMessage(group, "le erre, el café salió 1500") {
		t.Error("expected a match on 'café' regardless of message length")
	}
}

func TestMatchesMessage_StopwordOnlyOverlap_NoMatch(t *testing.T) {
	// Only 3-char/stopword tokens overlap — must not match.
	group := transactionGroup{Movements: []movement.Movement{{Description: strPtr("de la")}}}
	if matchesMessage(group, "borra el de la lista") {
		t.Error("expected no match on stopword-only overlap")
	}
}

func TestMatchesMessage_Amount(t *testing.T) {
	// Verify amount matching still works.
	amount := decimal.NewFromInt(1500)
	group := transactionGroup{Movements: []movement.Movement{{Amount: amount}}}
	if !matchesMessage(group, "fue 1500 pesos") {
		t.Error("expected a match on the amount '1500'")
	}
}

func TestMatchesMessage_Merchant(t *testing.T) {
	// Verify merchant token matching works.
	group := transactionGroup{Movements: []movement.Movement{{Merchant: strPtr("Carrefour")}}}
	if !matchesMessage(group, "el gasto en Carrefour fue mucho") {
		t.Error("expected a match on merchant 'Carrefour'")
	}
}

func TestMatchesMessage_NoMatch_EmptyDescription(t *testing.T) {
	group := transactionGroup{Movements: []movement.Movement{{Description: strPtr("")}}}
	if matchesMessage(group, "some message") {
		t.Error("expected no match on empty description")
	}
}

func TestMatchesMessage_NoMatch_NilDescription(t *testing.T) {
	group := transactionGroup{Movements: []movement.Movement{{Description: nil}}}
	if matchesMessage(group, "some message") {
		t.Error("expected no match on nil description")
	}
}

func TestMatchesMessage_CaseInsensitive(t *testing.T) {
	group := transactionGroup{Movements: []movement.Movement{{Description: strPtr("TRABAS")}}}
	if !matchesMessage(group, "quiero eliminar mi registro de trabas") {
		t.Error("expected case-insensitive match")
	}
}

func TestMatchesMessage_TransferWithAccount(t *testing.T) {
	// Verify that a transfer movement with an account is handled correctly.
	accountID := uint64(123)
	m := movement.Movement{
		AccountID:   &accountID,
		Description: strPtr("transferencia"),
		Amount:      decimal.NewFromInt(500),
		Currency:    currency.ARS,
	}
	group := transactionGroup{Movements: []movement.Movement{m}}
	if !matchesMessage(group, "la transferencia de 500") {
		t.Error("expected a match on the description token 'transferencia'")
	}
}

// fakeMovementRepoForResolve captures the since/until arguments to FindSimilarForUser.
type fakeMovementRepoForResolve struct {
	result          []movement.Movement
	capturedSince   time.Time
	capturedUntil   *time.Time
}

func (r *fakeMovementRepoForResolve) InsertBatch(ms []movement.Movement) error {
	return nil
}

func (r *fakeMovementRepoForResolve) SumAmountForAccount(accountID uint64) (decimal.Decimal, error) {
	return decimal.Zero, nil
}

func (r *fakeMovementRepoForResolve) ReplaceMovements(oldIDs []uint, newMovements []movement.Movement) error {
	return nil
}

func (r *fakeMovementRepoForResolve) FindSimilarForUser(userID uint64, query string, since time.Time, until *time.Time) ([]movement.Movement, error) {
	r.capturedSince = since
	r.capturedUntil = until
	return r.result, nil
}

func (r *fakeMovementRepoForResolve) SoftDeleteByIDs(ids []uint) error {
	return nil
}

func TestResolveCandidates_NoMentionedDate_UsesStartOfTodayArgentina(t *testing.T) {
	fake := &fakeMovementRepoForResolve{result: []movement.Movement{{Model: gorm.Model{ID: 1}, Description: strPtr("Nafta YPF")}}}
	c := &controller{movements: fake}

	candidates, err := c.resolveCandidates(42, "che, lo de la nafta ypf era otro monto", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("got %d candidates, want 1", len(candidates))
	}

	arg := time.FixedZone("ART", -3*60*60)
	now := time.Now().In(arg)
	wantSince := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, arg)
	if !fake.capturedSince.Equal(wantSince) {
		t.Errorf("since = %v, want start of today ART %v", fake.capturedSince, wantSince)
	}
	if fake.capturedUntil != nil {
		t.Errorf("until = %v, want nil (no dateTo mentioned)", fake.capturedUntil)
	}
}

func TestResolveCandidates_NoTextMatch_FallsBackToRecentWindow(t *testing.T) {
	// Message shares no token/amount with either stored movement — the
	// fallback must still offer them (the "¿cuál?" picker), not empty.
	fake := &fakeMovementRepoForResolve{result: []movement.Movement{
		{Model: gorm.Model{ID: 1}, Description: strPtr("Café")},
		{Model: gorm.Model{ID: 2}, Description: strPtr("Panadería")},
	}}
	c := &controller{movements: fake}

	candidates, err := c.resolveCandidates(42, "era 700", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("got %d candidates, want 2 (fallback to recent window)", len(candidates))
	}
}

func TestResolveCandidates_EmptyWindow_ReturnsNoCandidates(t *testing.T) {
	fake := &fakeMovementRepoForResolve{result: nil}
	c := &controller{movements: fake}

	candidates, err := c.resolveCandidates(42, "era 700", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("got %d candidates, want 0 (nothing in window)", len(candidates))
	}
}
