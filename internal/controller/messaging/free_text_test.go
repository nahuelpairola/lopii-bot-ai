package messaging

import (
	"context"
	"encoding/json"
	"testing"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/agent"
	"lopiibot.com/internal/chathistory"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/flow"
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
	classifyPairs       []orchestrator.Pair
	runFn               func(execute func(string, json.RawMessage) (string, error)) (string, error)
	runCalled           bool
	// updateCalled marca que se llegó a ResolveUpdate. Es una aserción NEGATIVA:
	// el camino estructurado existe para no hacer esa segunda llamada.
	updateCalled bool
	// queryRunFn deja que el test maneje el loop de QUERY igual que runFn maneja
	// el unificado: recibe el executor REAL, así que las tools de lectura corren
	// contra la base. Sin esto, AnswerQuery devuelve una respuesta fija y el test
	// no toca ni un dato.
	queryRunFn func(execute func(string, json.RawMessage) (string, error)) (string, error)
}

func (o *fakeFullOrchestrator) ClassifyCreate(ctx context.Context, text string, taxonomy []orchestrator.TaxonomyEntry, accounts []orchestrator.AccountOption, today string) (orchestrator.CreateResult, error) {
	return o.createResult, nil
}
func (o *fakeFullOrchestrator) ResolveUpdate(ctx context.Context, text string, candidate orchestrator.MovementCandidate, accounts []orchestrator.AccountOption) (orchestrator.UpdateResult, error) {
	o.updateCalled = true
	return o.updateResult, nil
}
func (o *fakeFullOrchestrator) ResolveDelete(ctx context.Context, text string, candidate orchestrator.MovementCandidate) (orchestrator.DeleteResult, error) {
	return o.deleteResult, nil
}
func (o *fakeFullOrchestrator) ClassifyOnboarding(ctx context.Context, text string) (orchestrator.OnboardingResult, error) {
	return o.onboardingResult, o.onboardingErr
}
func (o *fakeFullOrchestrator) AnswerQuery(ctx context.Context, systemPrompt, userText string, history []orchestrator.QueryTurn, tools []orchestrator.AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error) {
	if o.queryRunFn != nil {
		return o.queryRunFn(execute)
	}
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
	engine.Register(flow.NewSubcategorySetupFlow(subs))
	engine.Register(flow.NewCategoryMatchOfferFlow())
	engine.Register(flow.NewCategoryProposalConfirmFlow())
	// Desde que no hay router, TODO mensaje pasa por el loop, y el loop arma el
	// prompt con cuentas y taxonomía: un controller sin esas repos ahora es un
	// nil deref, no un test más chico.
	return &controller{
		orchestrator: orch, engine: engine, subcategories: subs,
		accounts:    &fakeAccountRepoFull{},
		movements:   &fakeMovementRepoFull{},
		chatHistory: stubChatHistory{},
	}, store
}

// manageSettingsRun programa un loop que abre la configuración del área dada.
// Sin router, ese es el único camino a los wizards de cuentas, categorías y
// recordatorio.
func manageSettingsRun(area string) func(func(string, json.RawMessage) (string, error)) (string, error) {
	return func(execute func(string, json.RawMessage) (string, error)) (string, error) {
		_, err := execute(orchestrator.ToolManageSettings, json.RawMessage(`{"area":"`+area+`"}`))
		return "", err
	}
}

func TestCreateCategory_MatchStartsMatchOfferFlow(t *testing.T) {
	existing := newSubForTest(9, "Otros", "Regalos / donaciones")
	subs := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{"Otros|Regalos / donaciones": existing}}
	orch := &fakeFullOrchestrator{runFn: manageSettingsRun(agent.SettingsAreaCategory), categoryResult: orchestrator.CategoryCreateResult{
		Match: &orchestrator.CategoryMatch{Category: "Otros", Subcategory: "Regalos / donaciones"},
	}}
	c, store := newCreateCategoryController(orch, subs)

	if err := c.handleFreeText(context.Background(), nil, 0, 1, "quiero categoría regalos"); err != nil {
		t.Fatalf("handleFreeText: %v", err)
	}

	if store.flowName != flow.CategoryMatchOfferFlowName {
		t.Errorf("flow = %q, want %q", store.flowName, flow.CategoryMatchOfferFlowName)
	}
}

func TestCreateCategory_ProposalStartsConfirmFlow(t *testing.T) {
	subs := &fakeSubcategoryRepoFull{categories: []string{"Alimentación"}}
	orch := &fakeFullOrchestrator{runFn: manageSettingsRun(agent.SettingsAreaCategory), categoryResult: orchestrator.CategoryCreateResult{
		Proposal: &orchestrator.CategoryProposal{Category: "Regalos", Subcategory: "Regalos", Icon: "🎁", Description: "Regalos a terceros."},
	}}
	c, store := newCreateCategoryController(orch, subs)

	c.handleFreeText(context.Background(), nil, 0, 1, "categoría regalos")

	if store.flowName != flow.CategoryProposalConfirmFlowName {
		t.Errorf("flow = %q, want %q", store.flowName, flow.CategoryProposalConfirmFlowName)
	}
	if conversation.StringOrEmpty(store.data["category_is_new"]) != "true" {
		t.Errorf("category_is_new = %q, want true (Regalos not in existing categories)", store.data["category_is_new"])
	}
}

func TestCreateCategory_ProposalDuplicateFallsToMatch(t *testing.T) {
	existing := newSubForTest(9, "Otros", "Regalos / donaciones")
	subs := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{"Otros|Regalos / donaciones": existing}}
	orch := &fakeFullOrchestrator{runFn: manageSettingsRun(agent.SettingsAreaCategory), categoryResult: orchestrator.CategoryCreateResult{
		Proposal: &orchestrator.CategoryProposal{Category: "Otros", Subcategory: "Regalos / donaciones", Icon: "🎁", Description: "x"},
	}}
	c, store := newCreateCategoryController(orch, subs)

	c.handleFreeText(context.Background(), nil, 0, 1, "categoría regalos")

	if store.flowName != flow.CategoryMatchOfferFlowName {
		t.Errorf("flow = %q, want %q (exact-duplicate proposal must offer the existing)", store.flowName, flow.CategoryMatchOfferFlowName)
	}
}

func TestCreateCategory_ReservedProposalFallsToWizard(t *testing.T) {
	subs := &fakeSubcategoryRepoFull{}
	orch := &fakeFullOrchestrator{runFn: manageSettingsRun(agent.SettingsAreaCategory), categoryResult: orchestrator.CategoryCreateResult{
		Proposal: &orchestrator.CategoryProposal{Category: "Sistema", Subcategory: "Cualquiera", Icon: "⚙️", Description: "x"},
	}}
	c, store := newCreateCategoryController(orch, subs)

	c.handleFreeText(context.Background(), nil, 0, 1, "categoría sistema")

	if store.flowName != flow.SubcategorySetupFlowName {
		t.Errorf("flow = %q, want %q (reserved proposal falls to wizard)", store.flowName, flow.SubcategorySetupFlowName)
	}
}

func TestCreateCategory_LLMErrorFallsToWizard(t *testing.T) {
	subs := &fakeSubcategoryRepoFull{}
	orch := &fakeFullOrchestrator{runFn: manageSettingsRun(agent.SettingsAreaCategory), categoryErr: context.Canceled}
	c, store := newCreateCategoryController(orch, subs)

	c.handleFreeText(context.Background(), nil, 0, 1, "categoría lo que sea")

	if store.flowName != flow.SubcategorySetupFlowName {
		t.Errorf("flow = %q, want %q (LLM error falls to wizard)", store.flowName, flow.SubcategorySetupFlowName)
	}
}

func newManageDispatchEngine() (*conversation.Engine, *fakeStoreForController) {
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(flow.NewAccountCreateFlow())
	engine.Register(flow.NewAccountManageFlow(fakeBalanceSummer{}))
	return engine, store
}

// (a) wants_new → the create flow (prefill-seeded).
func TestHandleFreeText_AccountManage_WantsNew_StartsCreate(t *testing.T) {
	orch := &fakeFullOrchestrator{
		runFn:               manageSettingsRun(agent.SettingsAreaAccount),
		accountManageResult: orchestrator.AccountManageResult{WantsNewAccount: true},
	}
	engine, store := newManageDispatchEngine()
	accs := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true)}}
	c := &controller{orchestrator: orch, engine: engine, accounts: accs,
		subcategories: &fakeSubcategoryRepoFull{}, movements: &fakeMovementRepoFull{}, chatHistory: stubChatHistory{}}

	c.handleFreeText(context.Background(), nil, 0, 1, "quiero crear una cuenta nueva")

	if store.flowName != flow.AccountCreateFlowName {
		t.Errorf("started flow = %q, want %q", store.flowName, flow.AccountCreateFlowName)
	}
}

// (b) matched to an existing account → manage flow, lands on the menu.
func TestHandleFreeText_AccountManage_Matched_StartsMenu(t *testing.T) {
	id := uint64(2)
	orch := &fakeFullOrchestrator{
		runFn:               manageSettingsRun(agent.SettingsAreaAccount),
		accountManageResult: orchestrator.AccountManageResult{MatchedAccountID: &id},
	}
	engine, store := newManageDispatchEngine()
	accs := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true), acct(2, currency.ARS, false)}}
	c := &controller{orchestrator: orch, engine: engine, accounts: accs,
		subcategories: &fakeSubcategoryRepoFull{}, movements: &fakeMovementRepoFull{}, chatHistory: stubChatHistory{}}

	c.handleFreeText(context.Background(), nil, 0, 1, "renombrá la segunda")

	if store.flowName != flow.AccountManageFlowName {
		t.Fatalf("started flow = %q, want %q", store.flowName, flow.AccountManageFlowName)
	}
	if store.stepName != flow.StepAccountManageMenu {
		t.Errorf("stepName = %q, want %q", store.stepName, flow.StepAccountManageMenu)
	}
}

// (c) no match → manage flow at the candidate picker.
func TestHandleFreeText_AccountManage_NoMatch_StartsPick(t *testing.T) {
	orch := &fakeFullOrchestrator{runFn: manageSettingsRun(agent.SettingsAreaAccount)} // zero result: nil id, no wants_new
	engine, store := newManageDispatchEngine()
	accs := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true), acct(2, currency.ARS, false)}}
	c := &controller{orchestrator: orch, engine: engine, accounts: accs,
		subcategories: &fakeSubcategoryRepoFull{}, movements: &fakeMovementRepoFull{}, chatHistory: stubChatHistory{}}

	c.handleFreeText(context.Background(), nil, 0, 1, "cambiá el monto")

	if store.flowName != flow.AccountManageFlowName || store.stepName != flow.StepAccountManagePick {
		t.Errorf("flow/step = %q/%q, want %q/%q", store.flowName, store.stepName, flow.AccountManageFlowName, flow.StepAccountManagePick)
	}
}

// (d) a hallucinated id (not in the user's list) is never trusted → pick.
func TestHandleFreeText_AccountManage_HallucinatedID_StartsPick(t *testing.T) {
	id := uint64(999)
	orch := &fakeFullOrchestrator{
		runFn:               manageSettingsRun(agent.SettingsAreaAccount),
		accountManageResult: orchestrator.AccountManageResult{MatchedAccountID: &id},
	}
	engine, store := newManageDispatchEngine()
	accs := &fakeAccountRepoFull{byUserID: []account.Account{acct(1, currency.ARS, true)}}
	c := &controller{orchestrator: orch, engine: engine, accounts: accs,
		subcategories: &fakeSubcategoryRepoFull{}, movements: &fakeMovementRepoFull{}, chatHistory: stubChatHistory{}}

	c.handleFreeText(context.Background(), nil, 0, 1, "renombrá esa")

	if store.stepName != flow.StepAccountManagePick {
		t.Errorf("stepName = %q, want %q (hallucinated id must not skip the pick)", store.stepName, flow.StepAccountManagePick)
	}
}

// (e) a user with no accounts skips Call 2 and goes straight to create.
func TestHandleFreeText_AccountManage_NoAccounts_StartsCreate(t *testing.T) {
	orch := &fakeFullOrchestrator{runFn: manageSettingsRun(agent.SettingsAreaAccount)}
	engine, store := newManageDispatchEngine()
	accs := &fakeAccountRepoFull{byUserID: nil}
	c := &controller{orchestrator: orch, engine: engine, accounts: accs,
		subcategories: &fakeSubcategoryRepoFull{}, movements: &fakeMovementRepoFull{}, chatHistory: stubChatHistory{}}

	c.handleFreeText(context.Background(), nil, 0, 1, "quiero modificar una cuenta")

	if store.flowName != flow.AccountCreateFlowName {
		t.Errorf("started flow = %q, want %q (no accounts → create)", store.flowName, flow.AccountCreateFlowName)
	}
}

// When the LLM returns nothing usable (no match, no proposal), CREATE_CATEGORY
// falls back to the classic wizard rather than a dead end.
func TestHandleFreeText_CreateCategory_FallsBackToWizard(t *testing.T) {
	subs := &fakeSubcategoryRepoFull{}
	orch := &fakeFullOrchestrator{runFn: manageSettingsRun(agent.SettingsAreaCategory)} // empty categoryResult

	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(flow.NewSubcategorySetupFlow(subs))
	engine.Register(flow.NewCategoryMatchOfferFlow())
	engine.Register(flow.NewCategoryProposalConfirmFlow())
	c := &controller{orchestrator: orch, engine: engine, subcategories: subs,
		accounts: &fakeAccountRepoFull{}, movements: &fakeMovementRepoFull{}, chatHistory: stubChatHistory{}}

	c.handleFreeText(context.Background(), nil, 0, 1, "quiero crear una categoría nueva")

	if store.flowName != flow.SubcategorySetupFlowName {
		t.Errorf("started flow = %q, want %q", store.flowName, flow.SubcategorySetupFlowName)
	}
	if store.stepName != flow.StepChooseMode {
		t.Errorf("stepName = %q, want %q", store.stepName, flow.StepChooseMode)
	}
}

func TestHandleFreeText_Help(t *testing.T) {
	metrics := &fakeMetricRepo{}
	// Sin router, "ayuda" llega al loop y el loop llama reply_help.
	orch := &fakeFullOrchestrator{runFn: func(execute func(string, json.RawMessage) (string, error)) (string, error) {
		_, err := execute(orchestrator.ToolReplyHelp, json.RawMessage(`{}`))
		return "", err
	}}
	c := &controller{orchestrator: orch, metrics: metrics,
		accounts: &fakeAccountRepoFull{}, subcategories: &fakeSubcategoryRepoFull{},
		movements: &fakeMovementRepoFull{}, chatHistory: stubChatHistory{}}

	err := c.handleFreeText(context.Background(), nil, 0, 1, "ayuda")
	if err != nil {
		t.Fatalf("handleFreeText: %v", err)
	}
	if len(metrics.logged) != 1 || metrics.logged[0].outcome != outcomeHelpShown {
		t.Fatalf("expected one %q log, got %+v", outcomeHelpShown, metrics.logged)
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
		"movements": movement.EncodeMovementRows([]movement.MovementRow{
			{Type: "expense", Amount: "1000", Currency: "ARS", Category: "Alimentación", Subcategory: "NoExiste", Date: "2026-07-02"},
		}),
		"old_movement_ids": conversation.EncodeStringSlice([]string{"42"}),
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
		"movements": movement.EncodeMovementRows([]movement.MovementRow{
			{Type: "expense", Amount: "1000", Currency: "ARS", Category: "Alimentación", Subcategory: "Café", Date: "2026-07-02"},
		}),
		"old_movement_ids": conversation.EncodeStringSlice([]string{"42"}),
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

func TestStartAccountCreate_OneAccount_SeedsNameAndBalance(t *testing.T) {
	orch := &fakeFullOrchestrator{onboardingResult: orchestrator.OnboardingResult{
		Accounts: []orchestrator.OnboardingAccountDraft{{Name: "Cedears", Currency: "USD", Balance: "1041265"}},
	}}
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(flow.NewAccountCreateFlow())
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
	if store.stepName != flow.StepAccountCreateAskName {
		t.Errorf("stepName = %q, want %q (prefill, not skip)", store.stepName, flow.StepAccountCreateAskName)
	}
}

func TestStartAccountCreate_NoAccounts_NoSeed(t *testing.T) {
	orch := &fakeFullOrchestrator{onboardingResult: orchestrator.OnboardingResult{}}
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(flow.NewAccountCreateFlow())
	c := &controller{orchestrator: orch, engine: engine}

	c.startAccountCreate(context.Background(), nil, 0, 1, "quiero crear una cuenta nueva")

	if _, ok := store.data["account_name"]; ok {
		t.Error("no account extracted → account_name must not be seeded")
	}
	if _, ok := store.data["account_balance"]; ok {
		t.Error("no account extracted → account_balance must not be seeded")
	}
	if store.stepName != flow.StepAccountCreateAskName {
		t.Errorf("stepName = %q, want %q", store.stepName, flow.StepAccountCreateAskName)
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
	engine.Register(flow.NewAccountCreateFlow())
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
	engine.Register(flow.NewAccountCreateFlow())
	c := &controller{orchestrator: orch, engine: engine}

	c.startAccountCreate(context.Background(), nil, 0, 1, "nueva cuenta Cripto")

	if store.data["account_name"] != "Cripto" {
		t.Errorf("account_name seed = %v, want %q", store.data["account_name"], "Cripto")
	}
	if _, ok := store.data["account_balance"]; ok {
		t.Error("unparseable balance must not be seeded")
	}
}

// classifyPairs es lo que devuelve el clasificador fake. Vacío = PENDING_REVIEW
// en todas las filas, que es exactamente lo que hace el real cuando falla.
func (o *fakeFullOrchestrator) ClassifyCategories(_ context.Context, _ string, rows []orchestrator.ClassifyRow, _ []orchestrator.TaxonomyEntry) []orchestrator.Pair {
	if o.classifyPairs != nil {
		return o.classifyPairs
	}
	out := make([]orchestrator.Pair, len(rows))
	for i := range out {
		out[i] = orchestrator.Pair{Category: constants.PendingReview}
	}
	return out
}

// Los tests de ruteo se borraron con el router. Su sujeto —"el intent X abre el
// flow Y"— dejo de existir: handleFreeText ya no decide nada, llama al loop y
// el loop elige una tool. Lo que aquellos tests cubrian ahora lo cubren, mejor,
// los escenarios multi-turno de conversation_harness_test.go, que asertan sobre
// la BASE y no sobre a que funcion se llamo.
