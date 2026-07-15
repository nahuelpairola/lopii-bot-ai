package messaging

import (
	"context"
	"encoding/json"
	"testing"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/queryhistory"
	"lopiibot.com/internal/subcategory"
)

// stubQueryHistory is a no-op queryHistoryRepository for tests that exercise
// handleQuery but don't care about the conversation thread itself.
type stubQueryHistory struct{}

func (stubQueryHistory) Recent(userID uint64) ([]queryhistory.Turn, error) { return nil, nil }
func (stubQueryHistory) Append(userID uint64, question, answer string) error { return nil }

type fakeFullOrchestrator struct {
	intent            orchestrator.Intent
	needsConfirmation bool
	createResult      orchestrator.CreateResult
	onboardingResult  orchestrator.OnboardingResult
	onboardingErr     error
	updateResult      orchestrator.UpdateResult
	deleteResult      orchestrator.DeleteResult
	intentErr         error
	queryAnswer       string
	queryErr          error
}

func (o *fakeFullOrchestrator) ClassifyIntent(ctx context.Context, text string) (orchestrator.IntentResult, error) {
	return orchestrator.IntentResult{Intent: o.intent, NeedsConfirmation: o.needsConfirmation}, o.intentErr
}
func (o *fakeFullOrchestrator) ClassifyCreate(ctx context.Context, text string, taxonomy []orchestrator.TaxonomyEntry, accounts []orchestrator.AccountOption, today string) (orchestrator.CreateResult, error) {
	return o.createResult, nil
}
func (o *fakeFullOrchestrator) ResolveUpdate(ctx context.Context, text string, candidate orchestrator.MovementCandidate, accounts []orchestrator.AccountOption) (orchestrator.UpdateResult, error) {
	return o.updateResult, nil
}
func (o *fakeFullOrchestrator) ResolveDelete(ctx context.Context, text string, candidate orchestrator.MovementCandidate) (orchestrator.DeleteResult, error) {
	return o.deleteResult, nil
}
func (o *fakeFullOrchestrator) ClassifyOnboarding(ctx context.Context, text string) (orchestrator.OnboardingResult, error) {
	return o.onboardingResult, o.onboardingErr
}
func (o *fakeFullOrchestrator) AnswerQuery(ctx context.Context, systemPrompt, userText string, history []orchestrator.QueryTurn, tools []orchestrator.AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error) {
	return o.queryAnswer, o.queryErr
}

func TestHandleFreeText_QueryRoutesToLoopAndResolves(t *testing.T) {
	orch := &fakeFullOrchestrator{intent: orchestrator.IntentQuery, queryAnswer: "Gastaste 5000 ARS en mayo."}
	f := &fakeMetricRepo{}
	c := &controller{orchestrator: orch, metrics: f, queryHistory: stubQueryHistory{}}

	c.handleFreeText(context.Background(), nil, 123, 1, "cuánto gasté en mayo")

	found := false
	for _, o := range f.resolved {
		if o == outcomeQueryAnswered {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a resolve of %q, got %v", outcomeQueryAnswered, f.resolved)
	}
}

func TestHandleFreeText_QueryFailureResolvesFailed(t *testing.T) {
	orch := &fakeFullOrchestrator{intent: orchestrator.IntentQuery, queryErr: context.Canceled}
	f := &fakeMetricRepo{}
	c := &controller{orchestrator: orch, metrics: f, queryHistory: stubQueryHistory{}}

	c.handleFreeText(context.Background(), nil, 123, 1, "consulta que falla")

	found := false
	for _, o := range f.resolved {
		if o == outcomeQueryFailed {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a resolve of %q, got %v", outcomeQueryFailed, f.resolved)
	}
}

func TestStartMovementCreate_NoGaps_InsertsDirectlyNoEngine(t *testing.T) {
	sub := newSubForTest(1, "Alimentación", "Café")
	subRepo := &fakeSubcategoryRepoFull{
		byCategoryAndSub: map[string]*subcategory.Subcategory{"Alimentación|Café": sub},
		all:              []subcategory.Subcategory{*sub},
	}
	accRepo := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true)}}
	movRepo := &fakeMovementRepoFull{balances: map[uint64]string{1: "1000000"}}
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
	c := &controller{movements: movRepo, orchestrator: orch, engine: engine, subcategories: &fakeSubcategoryRepoFull{}, accounts: &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true)}}}

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

func TestHandleFreeText_LogsPendingForQuery(t *testing.T) {
	metrics := &fakeMetricRepo{}
	orch := &fakeFullOrchestrator{intent: orchestrator.IntentQuery, queryAnswer: "Gastaste 5000."}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	c := &controller{orchestrator: orch, engine: engine, metrics: metrics, queryHistory: stubQueryHistory{}}

	c.handleFreeText(context.Background(), nil, 0, 1, "cuánto gasté este mes")

	// QUERY now logs a pending row at routing time, then resolves it at the
	// handler terminal (WIP=1) — no longer a direct terminal log.
	if len(metrics.logged) != 1 || metrics.logged[0].outcome != outcomePending {
		t.Fatalf("expected one pending query log, got %+v", metrics.logged)
	}
}

func TestStartMovementCreate_NoGaps_ResolvesInserted(t *testing.T) {
	sub := newSubForTest(1, "Alimentación", "Café")
	subRepo := &fakeSubcategoryRepoFull{
		byCategoryAndSub: map[string]*subcategory.Subcategory{"Alimentación|Café": sub},
		all:              []subcategory.Subcategory{*sub},
	}
	metrics := &fakeMetricRepo{}
	orch := &fakeFullOrchestrator{createResult: orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{
		{Type: "expense", Amount: "3000", Currency: "ARS", Category: "Alimentación", Subcategory: "Café", PaymentMethod: "cash", Description: "Café", Date: "2026-07-02"},
	}}}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	c := &controller{subcategories: subRepo, accounts: &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true)}}, movements: &fakeMovementRepoFull{balances: map[uint64]string{1: "1000000"}}, orchestrator: orch, engine: engine, metrics: metrics}

	c.startMovementCreate(context.Background(), nil, 0, 1, "café 3000 efectivo", false)

	if len(metrics.resolved) != 1 || metrics.resolved[0] != outcomeCreateInserted {
		t.Fatalf("expected resolve create_inserted, got %+v", metrics.resolved)
	}
}

func TestFinishMovementConfirmFlow_Rewrite_ResolvesRewrite(t *testing.T) {
	metrics := &fakeMetricRepo{}
	c := &controller{metrics: metrics}

	c.finishMovementConfirmFlow(context.Background(), nil, 0, conversation.Data{"choice": "rewrite"})

	if len(metrics.resolved) != 1 || metrics.resolved[0] != outcomeCreateRewrite {
		t.Fatalf("expected resolve create_rewrite, got %+v", metrics.resolved)
	}
}

func TestFinishMovementCreateFlow_Cancelled_ResolvesCancelled(t *testing.T) {
	metrics := &fakeMetricRepo{}
	c := &controller{metrics: metrics}

	c.finishMovementCreateFlow(context.Background(), nil, 0, conversation.Data{"cancelled": "true"})

	if len(metrics.resolved) != 1 || metrics.resolved[0] != outcomeCreateCancelled {
		t.Fatalf("expected resolve create_cancelled, got %+v", metrics.resolved)
	}
}

func TestFinishMovementUpdateConfirmFlow_Cancelled_ResolvesCancelled(t *testing.T) {
	metrics := &fakeMetricRepo{}
	c := &controller{metrics: metrics}

	c.finishMovementUpdateConfirmFlow(context.Background(), nil, 0, conversation.Data{"confirmed": "false"})

	if len(metrics.resolved) != 1 || metrics.resolved[0] != outcomeUpdateCancelled {
		t.Fatalf("expected resolve update_cancelled, got %+v", metrics.resolved)
	}
}

func TestFinishMovementUpdateConfirmFlow_Error_NilBotNoPanic(t *testing.T) {
	sub := newSubForTest(1, "Alimentación", "Café")
	movRepo := &fakeMovementRepoFull{}
	metrics := &fakeMetricRepo{}
	c := &controller{movements: movRepo, accounts: &fakeAccountRepoFull{}, subcategories: &fakeSubcategoryRepoFull{
		byCategoryAndSub: map[string]*subcategory.Subcategory{"Alimentación|Café": sub},
	}, metrics: metrics}

	data := conversation.Data{
		conversation.UserIDKey: uint64(1),
		"confirmed":            "true",
		"movements": encodeMovementRows([]movementRow{
			{Type: "expense", Amount: "1000", Currency: "ARS", Category: "Alimentación", Subcategory: "NoExiste", Date: "2026-07-02"},
		}),
		"old_movement_ids": encodeStringSlice([]string{"42"}),
	}

	// Must not panic with b == nil, even though resolveAndInsertMovements fails
	c.finishMovementUpdateConfirmFlow(context.Background(), nil, 123, data)
}

func TestFinishMovementUpdateConfirmFlow_Success_NilBotNoPanic(t *testing.T) {
	sub := newSubForTest(1, "Alimentación", "Café")
	movRepo := &fakeMovementRepoFull{}
	metrics := &fakeMetricRepo{}
	c := &controller{movements: movRepo, accounts: &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true)}}, subcategories: &fakeSubcategoryRepoFull{
		byCategoryAndSub: map[string]*subcategory.Subcategory{"Alimentación|Café": sub},
	}, metrics: metrics}

	data := conversation.Data{
		conversation.UserIDKey: uint64(1),
		"confirmed":            "true",
		"movements": encodeMovementRows([]movementRow{
			{Type: "expense", Amount: "1000", Currency: "ARS", Category: "Alimentación", Subcategory: "Café", Date: "2026-07-02"},
		}),
		"old_movement_ids": encodeStringSlice([]string{"42"}),
	}

	// Must not panic with b == nil, even though resolveAndInsertMovements succeeds and tries to send a message
	c.finishMovementUpdateConfirmFlow(context.Background(), nil, 123, data)
}

func TestFinishMovementDeleteFlow_Cancelled_ResolvesCancelled(t *testing.T) {
	metrics := &fakeMetricRepo{}
	c := &controller{metrics: metrics}

	c.finishMovementDeleteFlow(context.Background(), nil, 0, conversation.Data{"confirmed": "false"})

	if len(metrics.resolved) != 1 || metrics.resolved[0] != outcomeDeleteCancelled {
		t.Fatalf("expected resolve delete_cancelled, got %+v", metrics.resolved)
	}
}

func TestStartMovementDelete_NoCandidates_ResolvesNoCandidates(t *testing.T) {
	metrics := &fakeMetricRepo{}
	orch := &fakeFullOrchestrator{deleteResult: orchestrator.DeleteResult{Resolved: false}}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewMovementDeleteFlow())
	c := &controller{movements: &fakeMovementRepoFull{}, orchestrator: orch, engine: engine, subcategories: &fakeSubcategoryRepoFull{}, metrics: metrics}

	c.startMovementDelete(context.Background(), nil, 0, 1, "borrá lo de ayer")

	if len(metrics.resolved) != 1 || metrics.resolved[0] != outcomeNoCandidates {
		t.Fatalf("expected resolve no_candidates, got %+v", metrics.resolved)
	}
}

func TestStartAccountCreate_OneAccount_SeedsNameAndBalance(t *testing.T) {
	orch := &fakeFullOrchestrator{onboardingResult: orchestrator.OnboardingResult{
		Accounts: []orchestrator.OnboardingAccountDraft{{Name: "Cedears", Currency: "USD", Balance: "1041265"}},
	}}
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewAccountCreateFlow())
	c := &controller{orchestrator: orch, engine: engine}

	c.startAccountCreate(context.Background(), nil, 0, 1, "Nueva cuenta: Cedears tengo 1041265")

	if store.data["account_name"] != "Cedears" {
		t.Errorf("account_name seed = %v, want %q", store.data["account_name"], "Cedears")
	}
	if store.data["account_balance"] != "1041265" {
		t.Errorf("account_balance seed = %v, want %q", store.data["account_balance"], "1041265")
	}
	if _, ok := store.data["account_currency"]; ok {
		t.Error("account_currency must NOT be seeded (stays the currency ChoiceStep)")
	}
	if store.stepName != stepAccountCreateAskName {
		t.Errorf("stepName = %q, want %q (prefill, not skip)", store.stepName, stepAccountCreateAskName)
	}
}

func TestStartAccountCreate_NoAccounts_NoSeed(t *testing.T) {
	orch := &fakeFullOrchestrator{onboardingResult: orchestrator.OnboardingResult{}}
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewAccountCreateFlow())
	c := &controller{orchestrator: orch, engine: engine}

	c.startAccountCreate(context.Background(), nil, 0, 1, "quiero crear una cuenta nueva")

	if _, ok := store.data["account_name"]; ok {
		t.Error("no account extracted → account_name must not be seeded")
	}
	if _, ok := store.data["account_balance"]; ok {
		t.Error("no account extracted → account_balance must not be seeded")
	}
	if store.stepName != stepAccountCreateAskName {
		t.Errorf("stepName = %q, want %q", store.stepName, stepAccountCreateAskName)
	}
}

func TestStartAccountCreate_MultipleAccounts_NoSeed(t *testing.T) {
	orch := &fakeFullOrchestrator{onboardingResult: orchestrator.OnboardingResult{
		Accounts: []orchestrator.OnboardingAccountDraft{
			{Name: "Banco", Currency: "ARS", Balance: "1000"},
			{Name: "Efectivo", Currency: "ARS", Balance: "2000"},
		},
	}}
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewAccountCreateFlow())
	c := &controller{orchestrator: orch, engine: engine}

	c.startAccountCreate(context.Background(), nil, 0, 1, "tengo el banco con 1000 y efectivo 2000")

	if _, ok := store.data["account_name"]; ok {
		t.Error(">1 account (bulk onboarding misrouted) → seed nothing")
	}
}

func TestStartAccountCreate_GarbageBalance_SeedsNameOnly(t *testing.T) {
	orch := &fakeFullOrchestrator{onboardingResult: orchestrator.OnboardingResult{
		Accounts: []orchestrator.OnboardingAccountDraft{{Name: "Cripto", Currency: "USD", Balance: "no-es-numero"}},
	}}
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewAccountCreateFlow())
	c := &controller{orchestrator: orch, engine: engine}

	c.startAccountCreate(context.Background(), nil, 0, 1, "nueva cuenta Cripto")

	if store.data["account_name"] != "Cripto" {
		t.Errorf("account_name seed = %v, want %q", store.data["account_name"], "Cripto")
	}
	if _, ok := store.data["account_balance"]; ok {
		t.Error("unparseable balance must not be seeded")
	}
}
