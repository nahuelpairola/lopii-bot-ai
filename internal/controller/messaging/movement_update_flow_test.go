package messaging

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
)

type fakeOrchestrator struct {
	updateResult      orchestrator.UpdateResult
	updateErr         error
	gotUpdateAccounts []orchestrator.AccountOption
}

func (o *fakeOrchestrator) ClassifyIntent(ctx context.Context, text string) (orchestrator.IntentResult, error) {
	return orchestrator.IntentResult{}, nil
}
func (o *fakeOrchestrator) ClassifyCreate(ctx context.Context, text string, taxonomy []orchestrator.TaxonomyEntry, accounts []orchestrator.AccountOption, today string) (orchestrator.CreateResult, error) {
	return orchestrator.CreateResult{}, nil
}
func (o *fakeOrchestrator) ResolveUpdate(ctx context.Context, text string, candidate orchestrator.MovementCandidate, accounts []orchestrator.AccountOption) (orchestrator.UpdateResult, error) {
	o.gotUpdateAccounts = accounts
	return o.updateResult, o.updateErr
}
func (o *fakeOrchestrator) ResolveDelete(ctx context.Context, text string, candidate orchestrator.MovementCandidate) (orchestrator.DeleteResult, error) {
	return orchestrator.DeleteResult{}, nil
}
func (o *fakeOrchestrator) ClassifyOnboarding(ctx context.Context, text string) (orchestrator.OnboardingResult, error) {
	return orchestrator.OnboardingResult{}, nil
}
func (o *fakeOrchestrator) AnswerQuery(ctx context.Context, systemPrompt, userText string, history []orchestrator.QueryTurn, tools []orchestrator.AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error) {
	return "", nil
}
func (o *fakeOrchestrator) ClassifyCategoryCreate(ctx context.Context, text string, taxonomy []orchestrator.TaxonomyEntry) (orchestrator.CategoryCreateResult, error) {
	return orchestrator.CategoryCreateResult{}, nil
}

type fakeStoreForController struct {
	flowName, stepName string
	data               conversation.Data
	updatedAt          time.Time
	found              bool
}

func (s *fakeStoreForController) Get(userID uint64) (string, string, conversation.Data, time.Time, bool, error) {
	return s.flowName, s.stepName, s.data, s.updatedAt, s.found, nil
}
func (s *fakeStoreForController) Set(userID uint64, flowName, stepName string, data conversation.Data) error {
	s.flowName, s.stepName, s.data, s.found = flowName, stepName, data, true
	s.updatedAt = time.Now()
	return nil
}
func (s *fakeStoreForController) Clear(userID uint64) error {
	s.found = false
	return nil
}

func TestMovementToRow_ResolvesCategoryFromSubcategory(t *testing.T) {
	sub := newSubForTest(3, "Transporte", "Nafta")
	m := movement.Movement{SubcategoryID: 3, Subcategory: sub, Type: movement.Expense, Amount: mustDecimal(t, "15000"), Currency: "ARS"}

	row := movementToRow(m)
	if row.Category != "Transporte" || row.Subcategory != "Nafta" {
		t.Errorf("row category/subcategory = %q/%q, want Transporte/Nafta", row.Category, row.Subcategory)
	}
	if row.Amount != "15000" {
		t.Errorf("row amount = %q, want 15000", row.Amount)
	}
}

func TestMovementToRow_EmitsAbsAmount(t *testing.T) {
	id := uint64(1)
	m := movement.Movement{Type: movement.Expense, Amount: decimal.NewFromInt(-100000), Currency: currency.ARS, AccountID: &id, Date: time.Now()}
	row := movementToRow(m)
	if row.Amount != "100000" {
		t.Fatalf("row.Amount = %q, want positive 100000 (abs boundary — the LLM must never see the stored sign)", row.Amount)
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
	groups := []transactionGroup{
		{TransactionID: "", Movements: []movement.Movement{{SubcategoryID: 1, Subcategory: sub, Amount: mustDecimal(t, "3000"), Currency: "ARS"}}},
	}

	encoded := encodeCandidateGroups(groups)
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
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewMovementUpdateConfirmFlow())

	accRepo := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true), acct(7, currency.ARS, false)}}
	c := &controller{orchestrator: orch, engine: engine, subcategories: &fakeSubcategoryRepoFull{}, accounts: accRepo}

	beforeRows := []movementRow{{Type: "expense", Amount: "3000", Currency: "ARS", Category: "Alimentación", Subcategory: "Café"}}
	if err := c.proceedToUpdateConfirm(context.Background(), nil, 0, 1, "en realidad fue 3500", "", []string{"42"}, beforeRows); err != nil {
		t.Fatalf("proceedToUpdateConfirm: %v", err)
	}
	if store.flowName != movementUpdateConfirmFlowName {
		t.Errorf("started flow = %q, want %q", store.flowName, movementUpdateConfirmFlowName)
	}
	if len(orch.gotUpdateAccounts) == 0 {
		t.Error("ResolveUpdate should receive the user's accounts so it can re-target by name, got none")
	}
}

func TestProceedToUpdateConfirm_UnresolvedSendsNoDBCall(t *testing.T) {
	orch := &fakeOrchestrator{updateResult: orchestrator.UpdateResult{Resolved: false}}
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewMovementUpdateConfirmFlow())
	c := &controller{orchestrator: orch, engine: engine, accounts: &fakeAccountRepoFull{}}

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
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewMovementUpdateConfirmFlow())
	c := &controller{engine: engine, subcategories: &fakeSubcategoryRepoFull{}, accounts: &fakeAccountRepoFull{}}

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
