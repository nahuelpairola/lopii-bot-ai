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
	intent            orchestrator.Intent
	needsConfirmation bool
	createResult      orchestrator.CreateResult
	updateResult      orchestrator.UpdateResult
	deleteResult      orchestrator.DeleteResult
	intentErr         error
}

func (o *fakeFullOrchestrator) ClassifyIntent(ctx context.Context, text string) (orchestrator.IntentResult, error) {
	return orchestrator.IntentResult{Intent: o.intent, NeedsConfirmation: o.needsConfirmation}, o.intentErr
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
	orch := &fakeFullOrchestrator{createResult: orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{
		{Type: "expense", Amount: "3000", Currency: "ARS", Category: "Alimentación", Subcategory: "Café", PaymentMethod: "cash", Description: "Café", Date: "2026-07-02"},
	}}}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo, orchestrator: orch, engine: engine}

	c.startMovementCreate(context.Background(), nil, 0, 1, "café 3000 efectivo", false)

	if len(movRepo.inserted) != 1 {
		t.Fatalf("expected a direct insert with no gaps, got %d movements inserted", len(movRepo.inserted))
	}
	if store.found {
		t.Error("a gap-free CREATE should never touch the conversation engine")
	}
}

func TestStartMovementCreate_NeedsConfirmation_RoutesToConfirmGate(t *testing.T) {
	subRepo := &fakeSubcategoryRepoFull{}
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{}
	orch := &fakeFullOrchestrator{}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewMovementConfirmFlow())
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo, orchestrator: orch, engine: engine}

	c.startMovementCreate(context.Background(), nil, 0, 1, "20k", true)

	if store.flowName != movementConfirmFlowName {
		t.Errorf("started flow = %q, want %q (router's needs_confirmation should route to the confirm gate)", store.flowName, movementConfirmFlowName)
	}
}

func TestStartMovementCreate_WithGaps_StartsEngine(t *testing.T) {
	subRepo := &fakeSubcategoryRepoFull{categories: []string{"Alimentación"}}
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{}
	orch := &fakeFullOrchestrator{createResult: orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{
		{Type: "expense", Amount: "3000", Currency: "ARS", Category: "PENDING_REVIEW", Subcategory: "PENDING_REVIEW", Date: "2026-07-02"},
	}}}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewMovementCreateFlow(subRepo, accRepo))
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo, orchestrator: orch, engine: engine}

	c.startMovementCreate(context.Background(), nil, 0, 1, "gasté 3000 en algo", false)

	if len(movRepo.inserted) != 0 {
		t.Error("a CREATE with a category gap should not insert until the gap is filled")
	}
	if !store.found || store.stepName != stepResolveCategory {
		t.Errorf("expected the engine to be at %q, got found=%v step=%q", stepResolveCategory, store.found, store.stepName)
	}
}

func TestStartMovementUpdate_OneCandidate_ResolvesToConfirm(t *testing.T) {
	movRepo := &fakeMovementRepoFull{similar: []movement.Movement{
		{SubcategoryID: 1, Amount: mustDecimal(t, "3000"), Currency: "ARS", Description: strPtr("café")},
	}}
	orch := &fakeFullOrchestrator{updateResult: orchestrator.UpdateResult{
		Resolved: true,
		Movements: []orchestrator.MovementDraft{
			{Type: "expense", Amount: "3500", Currency: "ARS", Category: "Alimentación", Subcategory: "Café", Date: "2026-07-02"},
		},
	}}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewMovementUpdateConfirmFlow())
	c := &controller{movements: movRepo, orchestrator: orch, engine: engine, subcategories: &fakeSubcategoryRepoFull{}}

	c.startMovementUpdate(context.Background(), nil, 0, 1, "el café en realidad fue 3500")

	if store.flowName != movementUpdateConfirmFlowName {
		t.Errorf("started flow = %q, want %q (single resolveCandidates match should resolve to confirm)", store.flowName, movementUpdateConfirmFlowName)
	}
}

func TestStartMovementDelete_NoCandidates_SendsErrorNoFlow(t *testing.T) {
	movRepo := &fakeMovementRepoFull{}
	orch := &fakeFullOrchestrator{deleteResult: orchestrator.DeleteResult{Resolved: false}}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewMovementDeleteFlow())
	c := &controller{movements: movRepo, orchestrator: orch, engine: engine, subcategories: &fakeSubcategoryRepoFull{}}

	c.startMovementDelete(context.Background(), nil, 0, 1, "borrá lo de ayer")

	if store.found {
		t.Error("with zero candidates, no flow should ever start")
	}
}

func TestHandleFreeText_AccountCreate_StartsFlow(t *testing.T) {
	orch := &fakeFullOrchestrator{intent: orchestrator.IntentAccountCreate}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewAccountCreateFlow())
	c := &controller{orchestrator: orch, engine: engine}

	c.handleFreeText(context.Background(), nil, 0, 1, "quiero crear una cuenta nueva")

	if store.flowName != accountCreateFlowName {
		t.Errorf("started flow = %q, want %q", store.flowName, accountCreateFlowName)
	}
	if store.stepName != stepAccountCreateAskName {
		t.Errorf("stepName = %q, want %q", store.stepName, stepAccountCreateAskName)
	}
}

func TestHandleFreeText_CreateCategory_StartsFlow(t *testing.T) {
	orch := &fakeFullOrchestrator{intent: orchestrator.IntentCreateCategory}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewSubcategorySetupFlow(&fakeSubcategoryRepoFull{}))
	c := &controller{orchestrator: orch, engine: engine}

	c.handleFreeText(context.Background(), nil, 0, 1, "quiero crear una categoría nueva")

	if store.flowName != subcategorySetupFlowName {
		t.Errorf("started flow = %q, want %q", store.flowName, subcategorySetupFlowName)
	}
	if store.stepName != stepChooseMode {
		t.Errorf("stepName = %q, want %q", store.stepName, stepChooseMode)
	}
}

func TestHandleFreeText_LogsPendingForCreate(t *testing.T) {
	metrics := &fakeMetricRepo{}
	orch := &fakeFullOrchestrator{intent: orchestrator.IntentCreate, needsConfirmation: true}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewMovementConfirmFlow())
	c := &controller{orchestrator: orch, engine: engine, metrics: metrics}

	c.handleFreeText(context.Background(), nil, 0, 1, "20k")

	if len(metrics.logged) != 1 {
		t.Fatalf("expected one router log, got %d", len(metrics.logged))
	}
	if metrics.logged[0].outcome != outcomePending {
		t.Errorf("CREATE log outcome = %q, want %q", metrics.logged[0].outcome, outcomePending)
	}
}

func TestHandleFreeText_LogsTerminalForQuery(t *testing.T) {
	metrics := &fakeMetricRepo{}
	orch := &fakeFullOrchestrator{intent: orchestrator.IntentQuery}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	c := &controller{orchestrator: orch, engine: engine, metrics: metrics}

	c.handleFreeText(context.Background(), nil, 0, 1, "cuánto gasté este mes")

	if len(metrics.logged) != 1 || metrics.logged[0].outcome != outcomeQueryUnsupported {
		t.Fatalf("expected one query_unsupported log, got %+v", metrics.logged)
	}
}
