package messaging

import (
	"context"
	"encoding/json"
	"testing"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/chathistory"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/subcategory"
)

// stubChatHistory is a no-op chatHistoryRepository for tests that exercise
// handleQuery but don't care about the conversation thread itself.
type stubChatHistory struct{}

func (stubChatHistory) Recent(userID uint64) ([]chathistory.Turn, error)    { return nil, nil }
func (stubChatHistory) Append(userID uint64, question, answer string) error { return nil }

type fakeFullOrchestrator struct {
	intent              orchestrator.Intent
	createResult        orchestrator.CreateResult
	onboardingResult    orchestrator.OnboardingResult
	onboardingErr       error
	updateResult        orchestrator.UpdateResult
	deleteResult        orchestrator.DeleteResult
	intentErr           error
	queryAnswer         string
	queryErr            error
	categoryResult      orchestrator.CategoryCreateResult
	categoryErr         error
	accountManageResult orchestrator.AccountManageResult
	accountManageErr    error
	runFn               func(execute func(string, json.RawMessage) (string, error)) (string, error)
	runCalled           bool
}

func (o *fakeFullOrchestrator) ClassifyIntent(ctx context.Context, text string) (orchestrator.IntentResult, error) {
	return orchestrator.IntentResult{Intent: o.intent}, o.intentErr
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

// Run falla fuerte salvo que el test lo programe: sólo UPDATE y DELETE van por
// el loop en la etapa 2, así que llegar acá sin quererlo es haber migrado un
// camino antes de tiempo.
func (o *fakeFullOrchestrator) Run(_ context.Context, _, _ string, _ []orchestrator.QueryTurn, _ []orchestrator.AgentTool, execute func(string, json.RawMessage) (string, error)) (string, error) {
	o.runCalled = true
	if o.runFn == nil {
		return "", errRunNotWired
	}
	return o.runFn(swallowTurnDone(execute))
}
func (o *fakeFullOrchestrator) ClassifyCategoryCreate(ctx context.Context, text string, taxonomy []orchestrator.TaxonomyEntry) (orchestrator.CategoryCreateResult, error) {
	return o.categoryResult, o.categoryErr
}
func (o *fakeFullOrchestrator) ResolveAccountManage(ctx context.Context, text string, accounts []orchestrator.AccountOption) (orchestrator.AccountManageResult, error) {
	return o.accountManageResult, o.accountManageErr
}

// newCreateCategoryController wires a controller + engine with all three
// category flows registered, for the CREATE_CATEGORY dispatch tests.
func newCreateCategoryController(orch *fakeFullOrchestrator, subs *fakeSubcategoryRepoFull) (*controller, *fakeStoreForController) {
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewSubcategorySetupFlow(subs))
	engine.Register(NewCategoryMatchOfferFlow())
	engine.Register(NewCategoryProposalConfirmFlow())
	return &controller{orchestrator: orch, engine: engine, subcategories: subs}, store
}

func TestCreateCategory_MatchStartsMatchOfferFlow(t *testing.T) {
	existing := newSubForTest(9, "Otros", "Regalos / donaciones")
	subs := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{"Otros|Regalos / donaciones": existing}}
	orch := &fakeFullOrchestrator{intent: orchestrator.IntentCreateCategory, categoryResult: orchestrator.CategoryCreateResult{
		Match: &orchestrator.CategoryMatch{Category: "Otros", Subcategory: "Regalos / donaciones"},
	}}
	c, store := newCreateCategoryController(orch, subs)

	c.handleFreeText(context.Background(), nil, 0, 1, "quiero categoría regalos")

	if store.flowName != categoryMatchOfferFlowName {
		t.Errorf("flow = %q, want %q", store.flowName, categoryMatchOfferFlowName)
	}
}

func TestCreateCategory_ProposalStartsConfirmFlow(t *testing.T) {
	subs := &fakeSubcategoryRepoFull{categories: []string{"Alimentación"}}
	orch := &fakeFullOrchestrator{intent: orchestrator.IntentCreateCategory, categoryResult: orchestrator.CategoryCreateResult{
		Proposal: &orchestrator.CategoryProposal{Category: "Regalos", Subcategory: "Regalos", Icon: "🎁", Description: "Regalos a terceros."},
	}}
	c, store := newCreateCategoryController(orch, subs)

	c.handleFreeText(context.Background(), nil, 0, 1, "categoría regalos")

	if store.flowName != categoryProposalConfirmFlowName {
		t.Errorf("flow = %q, want %q", store.flowName, categoryProposalConfirmFlowName)
	}
	if stringOrEmpty(store.data["category_is_new"]) != "true" {
		t.Errorf("category_is_new = %q, want true (Regalos not in existing categories)", store.data["category_is_new"])
	}
}

func TestCreateCategory_ProposalDuplicateFallsToMatch(t *testing.T) {
	existing := newSubForTest(9, "Otros", "Regalos / donaciones")
	subs := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{"Otros|Regalos / donaciones": existing}}
	orch := &fakeFullOrchestrator{intent: orchestrator.IntentCreateCategory, categoryResult: orchestrator.CategoryCreateResult{
		Proposal: &orchestrator.CategoryProposal{Category: "Otros", Subcategory: "Regalos / donaciones", Icon: "🎁", Description: "x"},
	}}
	c, store := newCreateCategoryController(orch, subs)

	c.handleFreeText(context.Background(), nil, 0, 1, "categoría regalos")

	if store.flowName != categoryMatchOfferFlowName {
		t.Errorf("flow = %q, want %q (exact-duplicate proposal must offer the existing)", store.flowName, categoryMatchOfferFlowName)
	}
}

func TestCreateCategory_ReservedProposalFallsToWizard(t *testing.T) {
	subs := &fakeSubcategoryRepoFull{}
	orch := &fakeFullOrchestrator{intent: orchestrator.IntentCreateCategory, categoryResult: orchestrator.CategoryCreateResult{
		Proposal: &orchestrator.CategoryProposal{Category: "Sistema", Subcategory: "Cualquiera", Icon: "⚙️", Description: "x"},
	}}
	c, store := newCreateCategoryController(orch, subs)

	c.handleFreeText(context.Background(), nil, 0, 1, "categoría sistema")

	if store.flowName != subcategorySetupFlowName {
		t.Errorf("flow = %q, want %q (reserved proposal falls to wizard)", store.flowName, subcategorySetupFlowName)
	}
}

func TestCreateCategory_LLMErrorFallsToWizard(t *testing.T) {
	subs := &fakeSubcategoryRepoFull{}
	orch := &fakeFullOrchestrator{intent: orchestrator.IntentCreateCategory, categoryErr: context.Canceled}
	c, store := newCreateCategoryController(orch, subs)

	c.handleFreeText(context.Background(), nil, 0, 1, "categoría lo que sea")

	if store.flowName != subcategorySetupFlowName {
		t.Errorf("flow = %q, want %q (LLM error falls to wizard)", store.flowName, subcategorySetupFlowName)
	}
}

func TestHandleFreeText_QueryRoutesToLoopAndResolves(t *testing.T) {
	orch := &fakeFullOrchestrator{intent: orchestrator.IntentQuery, queryAnswer: "Gastaste 5000 ARS en mayo."}
	f := &fakeMetricRepo{}
	c := &controller{orchestrator: orch, metrics: f, chatHistory: stubChatHistory{}}

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
	c := &controller{orchestrator: orch, metrics: f, chatHistory: stubChatHistory{}}

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

	c.startMovementCreate(context.Background(), nil, 0, 1, "café 3000 efectivo")

	if len(movRepo.inserted) != 1 {
		t.Fatalf("expected a direct insert with no gaps, got %d movements inserted", len(movRepo.inserted))
	}
	if store.found {
		t.Error("a gap-free CREATE should never touch the conversation engine")
	}
}

func TestStartMovementCreate_WithGaps_StartsEngine(t *testing.T) {
	subRepo := &fakeSubcategoryRepoFull{categories: []string{"Alimentación"}}
	// an existing default account isolates this test to the category gap —
	// with zero accounts, lazy-create (stepCreateFirstAccount) takes priority.
	accRepo := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true)}}
	movRepo := &fakeMovementRepoFull{}
	orch := &fakeFullOrchestrator{createResult: orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{
		{Type: "expense", Amount: "3000", Currency: "ARS", Category: "PENDING_REVIEW", Subcategory: "PENDING_REVIEW", Date: "2026-07-02"},
	}}}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewMovementCreateFlow(subRepo, accRepo))
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo, orchestrator: orch, engine: engine}

	c.startMovementCreate(context.Background(), nil, 0, 1, "gasté 3000 en algo")

	if len(movRepo.inserted) != 0 {
		t.Error("a CREATE with a category gap should not insert until the gap is filled")
	}
	if !store.found || store.stepName != stepResolveCategory {
		t.Errorf("expected the engine to be at %q, got found=%v step=%q", stepResolveCategory, store.found, store.stepName)
	}
}

// TestMovementCreate_ZeroAccounts_AsksNameCreatesDefault covers the
// lazy-create híbrido: a user with zero accounts gets asked for the account
// name before anything else, and answering it creates a default account
// attributed to the movement. Uses an income row (not expense) to keep this
// test scoped to Task 4 (creation+default) — the zero-balance insufficient-
// funds skip is Task 5's concern (keySkipBalanceCheck), tested separately.
func TestMovementCreate_ZeroAccounts_AsksNameCreatesDefault(t *testing.T) {
	sub := newSubForTest(1, "Ingresos", "Sueldo")
	subRepo := &fakeSubcategoryRepoFull{
		byCategoryAndSub: map[string]*subcategory.Subcategory{"Ingresos|Sueldo": sub},
		all:              []subcategory.Subcategory{*sub},
	}
	accRepo := &fakeAccountRepoFull{}
	movRepo := &fakeMovementRepoFull{}
	orch := &fakeFullOrchestrator{createResult: orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{
		{Type: "income", Amount: "500", Currency: "ARS", Category: "Ingresos", Subcategory: "Sueldo", Date: "2026-07-02"},
	}}}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewMovementCreateFlow(subRepo, accRepo))
	c := &controller{subcategories: subRepo, accounts: accRepo, movements: movRepo, orchestrator: orch, engine: engine}

	if err := c.startMovementCreate(context.Background(), nil, 0, 1, "me pagaron 500"); err != nil {
		t.Fatalf("startMovementCreate: %v", err)
	}
	if !store.found || store.stepName != stepCreateFirstAccount {
		t.Fatalf("expected the flow at %q, got found=%v step=%q", stepCreateFirstAccount, store.found, store.stepName)
	}

	if _, found, err := engine.Handle(1, conversation.Input{Text: "Galicia"}); err != nil || !found {
		t.Fatalf("engine.Handle (name): found=%v err=%v", found, err)
	}
	result, found, err := engine.Handle(1, conversation.Input{CallbackData: optionBalanceLater})
	if err != nil || !found {
		t.Fatalf("engine.Handle (skip balance): found=%v err=%v", found, err)
	}
	if !result.Finished {
		t.Fatalf("expected the flow to finish after skipping the balance question (no other gaps), got Finished=false")
	}

	inserted, err := c.resolveAndInsertMovements(result.Data)
	if err != nil {
		t.Fatalf("resolveAndInsertMovements: %v", err)
	}
	if len(accRepo.inserted) != 1 || accRepo.inserted[0].Name != "Galicia" || !accRepo.inserted[0].IsDefault {
		t.Fatalf("expected 1 default account named Galicia, got %+v", accRepo.inserted)
	}
	if accRepo.inserted[0].Currency != currency.ARS {
		t.Errorf("created account currency = %q, want ARS", accRepo.inserted[0].Currency)
	}
	if len(inserted) != 1 || inserted[0].AccountID == nil {
		t.Fatalf("expected the movement to reference the newly created account, got %+v", inserted)
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

func newManageDispatchEngine() (*conversation.Engine, *fakeStoreForController) {
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewAccountCreateFlow())
	engine.Register(NewAccountManageFlow(fakeBalanceSummer{}))
	return engine, store
}

// (a) wants_new → the create flow (prefill-seeded).
func TestHandleFreeText_AccountManage_WantsNew_StartsCreate(t *testing.T) {
	orch := &fakeFullOrchestrator{
		intent:              orchestrator.IntentAccountManage,
		accountManageResult: orchestrator.AccountManageResult{WantsNewAccount: true},
	}
	engine, store := newManageDispatchEngine()
	accs := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true)}}
	c := &controller{orchestrator: orch, engine: engine, accounts: accs}

	c.handleFreeText(context.Background(), nil, 0, 1, "quiero crear una cuenta nueva")

	if store.flowName != accountCreateFlowName {
		t.Errorf("started flow = %q, want %q", store.flowName, accountCreateFlowName)
	}
}

// (b) matched to an existing account → manage flow, lands on the menu.
func TestHandleFreeText_AccountManage_Matched_StartsMenu(t *testing.T) {
	id := uint64(2)
	orch := &fakeFullOrchestrator{
		intent:              orchestrator.IntentAccountManage,
		accountManageResult: orchestrator.AccountManageResult{MatchedAccountID: &id},
	}
	engine, store := newManageDispatchEngine()
	accs := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true), acct(2, currency.ARS, false)}}
	c := &controller{orchestrator: orch, engine: engine, accounts: accs}

	c.handleFreeText(context.Background(), nil, 0, 1, "renombrá la segunda")

	if store.flowName != accountManageFlowName {
		t.Fatalf("started flow = %q, want %q", store.flowName, accountManageFlowName)
	}
	if store.stepName != stepAccountManageMenu {
		t.Errorf("stepName = %q, want %q", store.stepName, stepAccountManageMenu)
	}
}

// (c) no match → manage flow at the candidate picker.
func TestHandleFreeText_AccountManage_NoMatch_StartsPick(t *testing.T) {
	orch := &fakeFullOrchestrator{intent: orchestrator.IntentAccountManage} // zero result: nil id, no wants_new
	engine, store := newManageDispatchEngine()
	accs := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true), acct(2, currency.ARS, false)}}
	c := &controller{orchestrator: orch, engine: engine, accounts: accs}

	c.handleFreeText(context.Background(), nil, 0, 1, "cambiá el monto")

	if store.flowName != accountManageFlowName || store.stepName != stepAccountManagePick {
		t.Errorf("flow/step = %q/%q, want %q/%q", store.flowName, store.stepName, accountManageFlowName, stepAccountManagePick)
	}
}

// (d) a hallucinated id (not in the user's list) is never trusted → pick.
func TestHandleFreeText_AccountManage_HallucinatedID_StartsPick(t *testing.T) {
	id := uint64(999)
	orch := &fakeFullOrchestrator{
		intent:              orchestrator.IntentAccountManage,
		accountManageResult: orchestrator.AccountManageResult{MatchedAccountID: &id},
	}
	engine, store := newManageDispatchEngine()
	accs := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true)}}
	c := &controller{orchestrator: orch, engine: engine, accounts: accs}

	c.handleFreeText(context.Background(), nil, 0, 1, "renombrá esa")

	if store.stepName != stepAccountManagePick {
		t.Errorf("stepName = %q, want %q (hallucinated id must not skip the pick)", store.stepName, stepAccountManagePick)
	}
}

// (e) a user with no accounts skips Call 2 and goes straight to create.
func TestHandleFreeText_AccountManage_NoAccounts_StartsCreate(t *testing.T) {
	orch := &fakeFullOrchestrator{intent: orchestrator.IntentAccountManage}
	engine, store := newManageDispatchEngine()
	accs := &fakeAccountRepoFull{byUserID: nil}
	c := &controller{orchestrator: orch, engine: engine, accounts: accs}

	c.handleFreeText(context.Background(), nil, 0, 1, "quiero modificar una cuenta")

	if store.flowName != accountCreateFlowName {
		t.Errorf("started flow = %q, want %q (no accounts → create)", store.flowName, accountCreateFlowName)
	}
}

// When the LLM returns nothing usable (no match, no proposal), CREATE_CATEGORY
// falls back to the classic wizard rather than a dead end.
func TestHandleFreeText_CreateCategory_FallsBackToWizard(t *testing.T) {
	subs := &fakeSubcategoryRepoFull{}
	orch := &fakeFullOrchestrator{intent: orchestrator.IntentCreateCategory} // empty categoryResult

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewSubcategorySetupFlow(subs))
	engine.Register(NewCategoryMatchOfferFlow())
	engine.Register(NewCategoryProposalConfirmFlow())
	c := &controller{orchestrator: orch, engine: engine, subcategories: subs}

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
	sub := newSubForTest(1, "Alimentación", "Café")
	subRepo := &fakeSubcategoryRepoFull{
		byCategoryAndSub: map[string]*subcategory.Subcategory{"Alimentación|Café": sub},
		all:              []subcategory.Subcategory{*sub},
	}
	accRepo := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true)}}
	movRepo := &fakeMovementRepoFull{balances: map[uint64]string{1: "1000000"}}
	orch := &fakeFullOrchestrator{intent: orchestrator.IntentCreate, createResult: orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{
		{Type: "expense", Amount: "3000", Currency: "ARS", Category: "Alimentación", Subcategory: "Café", PaymentMethod: "cash", Description: "Café", Date: "2026-07-02"},
	}}}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	c := &controller{orchestrator: orch, engine: engine, metrics: metrics, subcategories: subRepo, accounts: accRepo, movements: movRepo}

	c.handleFreeText(context.Background(), nil, 0, 1, "café 3000 efectivo")

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
	c := &controller{orchestrator: orch, engine: engine, metrics: metrics, chatHistory: stubChatHistory{}}

	c.handleFreeText(context.Background(), nil, 0, 1, "cuánto gasté este mes")

	// QUERY now logs a pending row at routing time, then resolves it at the
	// handler terminal (WIP=1) — no longer a direct terminal log.
	if len(metrics.logged) != 1 || metrics.logged[0].outcome != outcomePending {
		t.Fatalf("expected one pending query log, got %+v", metrics.logged)
	}
}

func TestHandleFreeText_Help(t *testing.T) {
	metrics := &fakeMetricRepo{}
	orch := &fakeFullOrchestrator{intent: orchestrator.IntentHelp}
	c := &controller{orchestrator: orch, metrics: metrics}

	err := c.handleFreeText(context.Background(), nil, 0, 1, "ayuda")
	if err != nil {
		t.Fatalf("handleFreeText: %v", err)
	}
	if len(metrics.logged) != 1 || metrics.logged[0].outcome != outcomeHelpShown {
		t.Fatalf("expected one %q log, got %+v", outcomeHelpShown, metrics.logged)
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

	c.startMovementCreate(context.Background(), nil, 0, 1, "café 3000 efectivo")

	if len(metrics.resolved) != 1 || metrics.resolved[0] != outcomeCreateInserted {
		t.Fatalf("expected resolve create_inserted, got %+v", metrics.resolved)
	}
}

func TestStartMovementCreate_InsertFailure_IsReported(t *testing.T) {
	sub := newSubForTest(1, "Alimentación", "Café")
	subRepo := &fakeSubcategoryRepoFull{
		byCategoryAndSub: map[string]*subcategory.Subcategory{"Alimentación|Café": sub},
		all:              []subcategory.Subcategory{*sub},
	}
	metrics := &fakeMetricRepo{}
	movRepo := &fakeMovementRepoFull{balances: map[uint64]string{1: "1000000"}, insertErr: context.DeadlineExceeded}
	orch := &fakeFullOrchestrator{createResult: orchestrator.CreateResult{Movements: []orchestrator.MovementDraft{
		{Type: "expense", Amount: "3000", Currency: "ARS", Category: "Alimentación", Subcategory: "Café", PaymentMethod: "cash", Description: "Café", Date: "2026-07-02"},
	}}}

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	c := &controller{subcategories: subRepo, accounts: &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true)}}, movements: movRepo, orchestrator: orch, engine: engine, metrics: metrics}

	err := c.startMovementCreate(context.Background(), nil, 0, 1, "café 3000 efectivo")

	if err == nil {
		t.Fatal("startMovementCreate returned nil on insert failure; want error surfaced to the spine")
	}
	if len(metrics.resolved) != 1 || metrics.resolved[0] != outcomeCreateFailed {
		t.Errorf("resolved = %+v, want [%q]", metrics.resolved, outcomeCreateFailed)
	}
}

func TestStartMovementUpdate_RepoFailure_IsReported(t *testing.T) {
	movRepo := &fakeMovementRepoFull{similarErr: context.DeadlineExceeded}
	c := &controller{movements: movRepo, orchestrator: &fakeFullOrchestrator{}, subcategories: &fakeSubcategoryRepoFull{}}

	err := c.startMovementUpdate(context.Background(), nil, 0, 1, "el café en realidad fue 3500")

	if err == nil {
		t.Fatal("startMovementUpdate returned nil on repo failure; want error surfaced to the spine")
	}
}

func TestStartMovementDelete_RepoFailure_IsReported(t *testing.T) {
	movRepo := &fakeMovementRepoFull{similarErr: context.DeadlineExceeded}
	c := &controller{movements: movRepo, orchestrator: &fakeFullOrchestrator{}, subcategories: &fakeSubcategoryRepoFull{}}

	err := c.startMovementDelete(context.Background(), nil, 0, 1, "borrá lo de ayer")

	if err == nil {
		t.Fatal("startMovementDelete returned nil on repo failure; want error surfaced to the spine")
	}
}

func TestStartAccountManage_RepoFailure_IsReported(t *testing.T) {
	accs := &fakeAccountRepoFull{byUserIDErr: context.DeadlineExceeded}
	c := &controller{accounts: accs, orchestrator: &fakeFullOrchestrator{}}

	err := c.startAccountManage(context.Background(), nil, 0, 1, "cambiá el monto")

	if err == nil {
		t.Fatal("startAccountManage returned nil on repo failure; want error surfaced to the spine")
	}
}

func TestStartSubcategorySetup_RepoAndWizardFailure_IsReported(t *testing.T) {
	// FindAllForUser fails -> falls back to the wizard; the wizard flow isn't
	// registered either, so both layers fail and the error must surface.
	subs := &fakeSubcategoryRepoFull{allErr: context.DeadlineExceeded}
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	c := &controller{subcategories: subs, orchestrator: &fakeFullOrchestrator{}, engine: engine}

	err := c.startSubcategorySetup(context.Background(), nil, 0, 1, "categoría nueva")

	if err == nil {
		t.Fatal("startSubcategorySetup returned nil when both the LLM path and the wizard fallback failed")
	}
}

func TestStartAccountCreate_FlowNotRegistered_IsReported(t *testing.T) {
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" }) // account_create not registered
	c := &controller{orchestrator: &fakeFullOrchestrator{}, engine: engine}

	err := c.startAccountCreate(context.Background(), nil, 0, 1, "nueva cuenta")

	if err == nil {
		t.Fatal("startAccountCreate returned nil when the flow could not start; want error surfaced to the spine")
	}
}

func TestStartReminderSetup_FlowNotRegistered_IsReported(t *testing.T) {
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" }) // reminder_setup not registered
	c := &controller{engine: engine, reminders: &fakeReminderRepo{}}

	err := c.startReminderSetup(context.Background(), nil, 0, 1)

	if err == nil {
		t.Fatal("startReminderSetup returned nil when the flow could not start; want error surfaced to the spine")
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

// TestRedirectTargetFor cubre los mensajes reales de intent_events que el router
// mandó a UPDATE/DELETE siendo pedidos sobre una CUENTA o una CATEGORÍA. Sin esta
// red el flujo busca movimientos: o marca no_candidates, o —peor, porque
// resolveCandidates cae al fallback de los 5 más recientes— muestra un picker de
// movimientos que no tienen nada que ver, y el usuario abandona.
//
// La regla es deliberadamente conservadora: si el mensaje nombra un movimiento,
// NO se redirige, aunque también nombre una cuenta ("mover este movimiento a la
// cuenta de Mercado Pago" es un UPDATE legítimo).
func TestRedirectTargetFor(t *testing.T) {
	cases := []struct {
		message string
		want    string
	}{
		// pedidos sobre cuentas mal ruteados a UPDATE
		{"Quiero modificar el monto de la cuenta Wallet ARS", redirectAccount},
		{"Quiero dejar en cero algunas cuentas", redirectAccount},
		{"Quiero corregir lo que tengo en una cuenta", redirectAccount},
		{"Quiero modificar los valores de las cuentas", redirectAccount},
		{"Quisiera cambiar el nombre del Fondo común de inversión Balanz por FCI", ""},

		// pedidos sobre categorías mal ruteados a UPDATE/DELETE
		{"Quiero eliminar una categoría", redirectCategory},

		// legítimos: nombran un movimiento -> no se tocan
		{"Quiero mover este último movimiento a la cuenta de Mercado pago", ""},
		{"Quiero cambiar la categoría de un movimiento", ""},
		{"Perdon, el asado eran 15 mil", ""},
		{"La panaderia era 2k", ""},
		{"Le erre eran 1500", ""},
	}

	for _, tc := range cases {
		if got := redirectTargetFor(tc.message); got != tc.want {
			t.Errorf("redirectTargetFor(%q) = %q, want %q", tc.message, got, tc.want)
		}
	}
}

// TestStartMovementUpdate_AccountRequest_RedirectsToAccountManage: el repo tiene un
// movimiento reciente, así que SIN la red startMovementUpdate resolvería un
// candidato y arrancaría el flujo de update sobre un movimiento que el usuario
// nunca mencionó. Con la red, el pedido va al flujo de cuentas.
func TestStartMovementUpdate_AccountRequest_RedirectsToAccountManage(t *testing.T) {
	movRepo := &fakeMovementRepoFull{similar: []movement.Movement{
		{SubcategoryID: 1, Amount: mustDecimal(t, "3000"), Currency: "ARS", Description: strPtr("café")},
	}}
	orch := &fakeFullOrchestrator{accountManageResult: orchestrator.AccountManageResult{WantsNewAccount: true}}
	engine, store := newManageDispatchEngine()
	c := &controller{movements: movRepo, orchestrator: orch, engine: engine,
		subcategories: &fakeSubcategoryRepoFull{},
		accounts:      &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true)}}}

	c.startMovementUpdate(context.Background(), nil, 0, 1, "Quiero dejar en cero algunas cuentas")

	if store.flowName == movementUpdatePickFlowName || store.flowName == movementUpdateConfirmFlowName {
		t.Fatalf("started flow = %q: un pedido sobre cuentas no debe abrir el flujo de movimientos", store.flowName)
	}
	if store.flowName != accountCreateFlowName {
		t.Errorf("started flow = %q, want %q", store.flowName, accountCreateFlowName)
	}
}
