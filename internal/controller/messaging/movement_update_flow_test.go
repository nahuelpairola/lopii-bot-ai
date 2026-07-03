package messaging

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/subcategory"
)

type fakeOrchestrator struct {
	updateResult orchestrator.UpdateResult
	updateErr    error
}

func (o *fakeOrchestrator) ClassifyIntent(ctx context.Context, text string) (orchestrator.Intent, error) {
	return "", nil
}
func (o *fakeOrchestrator) ClassifyCreate(ctx context.Context, text string, taxonomy []orchestrator.TaxonomyEntry, accounts []orchestrator.AccountOption, today string) (orchestrator.CreateResult, error) {
	return orchestrator.CreateResult{}, nil
}
func (o *fakeOrchestrator) ResolveUpdate(ctx context.Context, text string, candidate orchestrator.MovementCandidate) (orchestrator.UpdateResult, error) {
	return o.updateResult, o.updateErr
}
func (o *fakeOrchestrator) ResolveDelete(ctx context.Context, text string, candidate orchestrator.MovementCandidate) (orchestrator.DeleteResult, error) {
	return orchestrator.DeleteResult{}, nil
}

type fakeStoreForController struct {
	flowName, stepName string
	data               conversation.Data
	found              bool
}

func (s *fakeStoreForController) Get(userID uint64) (string, string, conversation.Data, bool, error) {
	return s.flowName, s.stepName, s.data, s.found, nil
}
func (s *fakeStoreForController) Set(userID uint64, flowName, stepName string, data conversation.Data) error {
	s.flowName, s.stepName, s.data, s.found = flowName, stepName, data, true
	return nil
}
func (s *fakeStoreForController) Clear(userID uint64) error {
	s.found = false
	return nil
}

func TestBuildSubcategoryIndex(t *testing.T) {
	sub := newSubForTest(7, "Alimentación", "Café")
	idx := buildSubcategoryIndex([]subcategory.Subcategory{*sub})
	if idx[7].Category != "Alimentación" {
		t.Errorf("index[7].Category = %q, want Alimentación", idx[7].Category)
	}
}

func TestMovementToRow_ResolvesCategoryFromIndex(t *testing.T) {
	sub := newSubForTest(3, "Transporte", "Nafta")
	idx := buildSubcategoryIndex([]subcategory.Subcategory{*sub})
	m := movement.Movement{SubcategoryID: 3, Type: movement.Expense, Amount: mustDecimal(t, "15000"), Currency: "ARS"}

	row := movementToRow(m, idx)
	if row.Category != "Transporte" || row.Subcategory != "Nafta" {
		t.Errorf("row category/subcategory = %q/%q, want Transporte/Nafta", row.Category, row.Subcategory)
	}
	if row.Amount != "15000" {
		t.Errorf("row amount = %q, want 15000", row.Amount)
	}
}

func TestRowToDraft_RoundTripsAccountID(t *testing.T) {
	row := movementRow{Type: "transfer", Amount: "100", Currency: "USD", AccountID: "5"}
	draft := rowToDraft(row)
	if draft.AccountID == nil || *draft.AccountID != 5 {
		t.Errorf("draft.AccountID = %v, want 5", draft.AccountID)
	}
}

func TestDraftToRow_RoundTripsAccountID(t *testing.T) {
	id := uint64(9)
	row := draftToRow(orchestrator.MovementDraft{Type: "transfer", Amount: "100", Currency: "USD", AccountID: &id})
	if row.AccountID != "9" {
		t.Errorf("row.AccountID = %q, want 9", row.AccountID)
	}
}

func TestEncodeDecodeCandidateGroups_RoundTrip(t *testing.T) {
	sub := newSubForTest(1, "Alimentación", "Nafta")
	idx := buildSubcategoryIndex([]subcategory.Subcategory{*sub})
	groups := []transactionGroup{
		{TransactionID: "", Movements: []movement.Movement{{SubcategoryID: 1, Amount: mustDecimal(t, "3000"), Currency: "ARS"}}},
	}

	encoded := encodeCandidateGroups(groups, idx)
	decoded := decodeCandidateGroups(conversation.Data{"candidate_groups": encoded})

	if len(decoded) != 1 {
		t.Fatalf("got %d candidates, want 1", len(decoded))
	}
	if len(decoded[0].Rows) != 1 || decoded[0].Rows[0].Amount != "3000" {
		t.Errorf("decoded rows = %+v, want amount 3000", decoded[0].Rows)
	}
}

func TestProceedToUpdateConfirm_SeedsConfirmFlowOnResolved(t *testing.T) {
	orch := &fakeOrchestrator{updateResult: orchestrator.UpdateResult{
		Resolved: true,
		Movements: []orchestrator.MovementDraft{
			{Type: "expense", Amount: "3500", Currency: "ARS", Category: "Alimentación", Subcategory: "Café", Description: "Café", Date: "2026-07-02"},
		},
	}}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store)
	engine.Register(NewMovementUpdateConfirmFlow())

	c := &controller{orchestrator: orch, engine: engine}

	beforeRows := []movementRow{{Type: "expense", Amount: "3000", Currency: "ARS", Category: "Alimentación", Subcategory: "Café"}}
	if err := c.proceedToUpdateConfirm(context.Background(), nil, 0, 1, "en realidad fue 3500", "", []string{"42"}, beforeRows); err != nil {
		t.Fatalf("proceedToUpdateConfirm: %v", err)
	}
	if store.flowName != movementUpdateConfirmFlowName {
		t.Errorf("started flow = %q, want %q", store.flowName, movementUpdateConfirmFlowName)
	}
}

func TestProceedToUpdateConfirm_UnresolvedSendsNoDBCall(t *testing.T) {
	orch := &fakeOrchestrator{updateResult: orchestrator.UpdateResult{Resolved: false}}
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store)
	engine.Register(NewMovementUpdateConfirmFlow())
	c := &controller{orchestrator: orch, engine: engine}

	if err := c.proceedToUpdateConfirm(context.Background(), nil, 0, 1, "che no sé", "", nil, nil); err != nil {
		t.Fatalf("proceedToUpdateConfirm: %v", err)
	}
	if store.found {
		t.Error("an unresolved result should never start the confirm flow")
	}
}

func TestSeedAndStartUpdateConfirm_NeverCallsOrchestrator(t *testing.T) {
	// orchestrator is deliberately nil — this function must not call it.
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store)
	engine.Register(NewMovementUpdateConfirmFlow())
	c := &controller{engine: engine}

	result := orchestrator.UpdateResult{Resolved: true, Movements: []orchestrator.MovementDraft{
		{Type: "expense", Amount: "3500", Currency: "ARS", Category: "Alimentación", Subcategory: "Café", Date: "2026-07-02"},
	}}
	if err := c.seedAndStartUpdateConfirm(context.Background(), nil, 0, 1, []string{"7"}, nil, result); err != nil {
		t.Fatalf("seedAndStartUpdateConfirm: %v", err)
	}
	if store.flowName != movementUpdateConfirmFlowName {
		t.Errorf("started flow = %q, want %q", store.flowName, movementUpdateConfirmFlowName)
	}
}

func mustDecimal(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.NewFromString(s)
	if err != nil {
		t.Fatalf("decimal.NewFromString(%q): %v", s, err)
	}
	return d
}
