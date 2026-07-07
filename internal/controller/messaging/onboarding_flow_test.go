package messaging

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/subcategory"
)

type fakeOrchestratorForOnboarding struct {
	onboardingResult orchestrator.OnboardingResult
	classifyErr      error
}

func (o *fakeOrchestratorForOnboarding) ClassifyOnboarding(ctx context.Context, text string) (orchestrator.OnboardingResult, error) {
	return o.onboardingResult, o.classifyErr
}
func (o *fakeOrchestratorForOnboarding) ClassifyIntent(ctx context.Context, text string) (orchestrator.IntentResult, error) {
	return orchestrator.IntentResult{}, nil
}
func (o *fakeOrchestratorForOnboarding) ClassifyCreate(ctx context.Context, text string, taxonomy []orchestrator.TaxonomyEntry, accounts []orchestrator.AccountOption, today string) (orchestrator.CreateResult, error) {
	return orchestrator.CreateResult{}, nil
}
func (o *fakeOrchestratorForOnboarding) ResolveUpdate(ctx context.Context, text string, candidate orchestrator.MovementCandidate) (orchestrator.UpdateResult, error) {
	return orchestrator.UpdateResult{}, nil
}
func (o *fakeOrchestratorForOnboarding) ResolveDelete(ctx context.Context, text string, candidate orchestrator.MovementCandidate) (orchestrator.DeleteResult, error) {
	return orchestrator.DeleteResult{}, nil
}

type fakeSubcategories struct {
	sub *subcategory.Subcategory
	err error
}

func (s *fakeSubcategories) FindByCategoryAndSubcategory(userID uint64, category, subcategory string) (*subcategory.Subcategory, error) {
	return s.sub, s.err
}
func (s *fakeSubcategories) FindAllForUser(userID uint64) ([]subcategory.Subcategory, error) {
	return nil, nil
}
func (s *fakeSubcategories) DistinctCategoriesForUser(userID uint64) ([]string, error) {
	return nil, nil
}
func (s *fakeSubcategories) IconForCategory(userID uint64, category string) string {
	return ""
}
func (s *fakeSubcategories) Insert(sub *subcategory.Subcategory) error {
	return nil
}
func (s *fakeSubcategories) Reload() error {
	return nil
}

type fakeMovements struct {
	openings []movement.AccountOpening
	err      error
}

func (m *fakeMovements) InsertAccountsWithOpenings(items []movement.AccountOpening) error {
	if m.err != nil {
		return m.err
	}
	m.openings = items
	return nil
}
func (m *fakeMovements) InsertBatch([]movement.Movement) error {
	return nil
}
func (m *fakeMovements) SumAmountForAccount(accountID uint64) (decimal.Decimal, error) {
	return decimal.Zero, nil
}
func (m *fakeMovements) ReplaceMovements(oldIDs []uint, newMovements []movement.Movement) error {
	return nil
}
func (m *fakeMovements) FindSimilarForUser(userID uint64, query string, since time.Time, until *time.Time) ([]movement.Movement, error) {
	return nil, nil
}
func (m *fakeMovements) SoftDeleteByIDs(ids []uint) error {
	return nil
}

func TestOnboardingRowsFromDrafts_DefaultsAndSkips(t *testing.T) {
	drafts := []orchestrator.OnboardingAccountDraft{
		{Name: "Banco", Currency: "ARS", Balance: "20000"},
		{Name: "", Currency: "", Balance: "5000"},        // → name Efectivo, currency ARS
		{Name: "Bróker", Currency: "USD", Balance: "100"},
		{Name: "Acciones", Currency: "USD", Balance: ""}, // no balance → skipped
	}
	rows := onboardingRowsFromDrafts(drafts)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3 (unparseable-balance row skipped)", len(rows))
	}
	if rows[1].Name != "Efectivo" || rows[1].Currency != "ARS" {
		t.Errorf("row 1 = %+v, want Efectivo/ARS defaults", rows[1])
	}
}

func TestMarkDefaults_FirstPerCurrency(t *testing.T) {
	rows := markDefaults([]onboardingRow{
		{Name: "Banco", Currency: "ARS"},
		{Name: "Efectivo", Currency: "ARS"},
		{Name: "Bróker", Currency: "USD"},
	})
	if rows[0].IsDefault != "true" || rows[1].IsDefault != "false" || rows[2].IsDefault != "true" {
		t.Errorf("defaults = %q/%q/%q, want true/false/true", rows[0].IsDefault, rows[1].IsDefault, rows[2].IsDefault)
	}
}

func TestEncodeDecodeOnboardingRows_RoundTrip(t *testing.T) {
	rows := []onboardingRow{{Name: "Banco", Currency: "ARS", Balance: "20000", IsDefault: "true"}}
	data := conversation.Data{"accounts": encodeOnboardingRows(rows)}
	got := decodeOnboardingRows(data)
	if len(got) != 1 || got[0].Name != "Banco" || got[0].IsDefault != "true" {
		t.Errorf("round trip = %+v, want the original row", got)
	}
}

func TestFinishOnboardingConfirmFlow_ConfirmInserts(t *testing.T) {
	subs := &fakeSubcategories{sub: &subcategory.Subcategory{Model: gorm.Model{ID: 7}, Category: "Sistema", Subcategory: "Saldo inicial"}}
	movs := &fakeMovements{}
	c := &controller{subcategories: subs, movements: movs}

	rows := markDefaults([]onboardingRow{
		{Name: "Banco", Currency: "ARS", Balance: "20000"},
		{Name: "Bróker", Currency: "USD", Balance: "100"},
	})
	data := conversation.Data{conversation.UserIDKey: uint64(1), "accounts": encodeOnboardingRows(rows)}

	c.finishOnboardingConfirmFlow(context.Background(), nil, 0, data)

	if len(movs.openings) != 2 {
		t.Fatalf("inserted %d account-openings, want 2", len(movs.openings))
	}
	if !movs.openings[0].Account.IsDefault || !movs.openings[1].Account.IsDefault {
		t.Errorf("default flags wrong: first-ARS should be default, USD is a different currency so also default")
	}
}
