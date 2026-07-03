package messaging

import (
	"context"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/subcategory"
)

type fakeFullOrchestrator struct {
	intent       orchestrator.Intent
	createResult orchestrator.CreateResult
	updateResult orchestrator.UpdateResult
	deleteResult orchestrator.DeleteResult
	intentErr    error
}

func (o *fakeFullOrchestrator) ClassifyIntent(ctx context.Context, text string) (orchestrator.Intent, error) {
	return o.intent, o.intentErr
}
func (o *fakeFullOrchestrator) ClassifyCreate(ctx context.Context, text string, taxonomy []orchestrator.TaxonomyEntry, accounts []orchestrator.AccountOption, today string) (orchestrator.CreateResult, error) {
	return o.createResult, nil
}
func (o *fakeFullOrchestrator) ResolveUpdate(ctx context.Context, text string, candidate orchestrator.MovementCandidate) (orchestrator.UpdateResult, error) {
	return o.updateResult, nil
}
func (o *fakeFullOrchestrator) ResolveDelete(ctx context.Context, text string, candidate orchestrator.MovementCandidate) (orchestrator.DeleteResult, error) {
	return o.deleteResult, nil
}

func TestStartMovementCreate_NoGaps_InsertsDirectlyNoEngine(t *testing.T) {
	sub := newSubForTest(1, "Alimentación", "Café")
	subRepo := &fakeSubcategoryRepoFull{
		byCategoryAndSub: map[string]*subcategory.Subcategory{"Alimentación|Café": sub},
		all:              []subcategory.Subcategory{*sub},
	}
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{}
	lastTx := &fakeLastTransactionStore{}
	orch := &fakeFullOrchestrator{createResult: orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{
		{Type: "expense", Amount: "3000", Currency: "ARS", Category: "Alimentación", Subcategory: "Café", PaymentMethod: "cash", Description: "Café", Date: "2026-07-02"},
	}}}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store)
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo, lastTransactions: lastTx, orchestrator: orch, engine: engine}

	c.startMovementCreate(context.Background(), nil, 0, 1, "café 3000 efectivo")

	if len(movRepo.inserted) != 1 {
		t.Fatalf("expected a direct insert with no gaps, got %d movements inserted", len(movRepo.inserted))
	}
	if store.found {
		t.Error("a gap-free CREATE should never touch the conversation engine")
	}
	if lastTx.set != 1 {
		t.Error("lastTransactions.Set should be called after a successful CREATE")
	}
}

func TestStartMovementCreate_WithGaps_StartsEngine(t *testing.T) {
	subRepo := &fakeSubcategoryRepoFull{categories: []string{"Alimentación"}}
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{}
	lastTx := &fakeLastTransactionStore{}
	orch := &fakeFullOrchestrator{createResult: orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{
		{Type: "expense", Amount: "3000", Currency: "ARS", Category: "PENDING_REVIEW", Subcategory: "PENDING_REVIEW", Date: "2026-07-02"},
	}}}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store)
	engine.Register(NewMovementCreateFlow(subRepo, accRepo))
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo, lastTransactions: lastTx, orchestrator: orch, engine: engine}

	c.startMovementCreate(context.Background(), nil, 0, 1, "gasté 3000 en algo")

	if len(movRepo.inserted) != 0 {
		t.Error("a CREATE with a category gap should not insert until the gap is filled")
	}
	if !store.found || store.stepName != stepResolveCategory {
		t.Errorf("expected the engine to be at %q, got found=%v step=%q", stepResolveCategory, store.found, store.stepName)
	}
}

func TestStartMovementUpdate_LastTransactionResolves_NoDBSearch(t *testing.T) {
	movRepo := &fakeMovementRepoFull{}
	lastTx := &fakeLastTransactionStore{}
	lastTx.stored = []movement.Movement{{SubcategoryID: 1, Amount: mustDecimal(t, "3000"), Currency: "ARS"}}
	orch := &fakeFullOrchestrator{updateResult: orchestrator.UpdateResult{
		Resolved: true,
		Movements: []orchestrator.MovementDraft{
			{Type: "expense", Amount: "3500", Currency: "ARS", Category: "Alimentación", Subcategory: "Café", Date: "2026-07-02"},
		},
	}}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store)
	engine.Register(NewMovementUpdateConfirmFlow())
	c := &controller{movements: movRepo, lastTransactions: lastTx, orchestrator: orch, engine: engine, subcategories: &fakeSubcategoryRepoFull{}}

	c.startMovementUpdate(context.Background(), nil, 0, 1, "en realidad fue 3500")

	if store.flowName != movementUpdateConfirmFlowName {
		t.Errorf("started flow = %q, want %q (lastTransaction should resolve directly)", store.flowName, movementUpdateConfirmFlowName)
	}
}

func TestStartMovementDelete_NoCandidates_SendsErrorNoFlow(t *testing.T) {
	movRepo := &fakeMovementRepoFull{}
	lastTx := &fakeLastTransactionStore{}
	orch := &fakeFullOrchestrator{deleteResult: orchestrator.DeleteResult{Resolved: false}}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store)
	engine.Register(NewMovementDeleteFlow())
	c := &controller{movements: movRepo, lastTransactions: lastTx, orchestrator: orch, engine: engine, subcategories: &fakeSubcategoryRepoFull{}}

	c.startMovementDelete(context.Background(), nil, 0, 1, "borrá lo de ayer")

	if store.found {
		t.Error("with zero candidates, no flow should ever start")
	}
}
