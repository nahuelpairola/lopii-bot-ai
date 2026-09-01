package messaging

import (
	"context"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/subcategory"
)

type fakeTargetLister struct {
	all []subcategory.Subcategory
}

func (f fakeTargetLister) FindAllForUser(uint64) ([]subcategory.Subcategory, error) {
	return f.all, nil
}

func (f fakeTargetLister) DistinctCategoriesForUser(uint64) ([]string, error) {
	seen := map[string]bool{}
	var cats []string
	for _, s := range f.all {
		if subcategory.IsReserved(s.Category) || seen[s.Category] {
			continue
		}
		seen[s.Category] = true
		cats = append(cats, s.Category)
	}
	return cats, nil
}

func (f fakeTargetLister) IconForCategory(_ uint64, category string) string {
	for _, s := range f.all {
		if s.Category == category {
			return flow.SubcategoryIcon(s)
		}
	}
	return flow.DefaultCategoryIcon
}

func targetCatalog() []subcategory.Subcategory {
	return []subcategory.Subcategory{
		ownedSub(7, "Comida", "Delivery", "🍕"),
		ownedSub(3, "Alimentos", "Delivery", "🥑"),
		ownedSub(4, "Alimentos", "Supermercado", "🛒"),
		ownedSub(1, "Sistema", "Saldo inicial", "⚙️"),
	}
}

func newTargetEngine() (*conversation.Engine, *fakeStateStore) {
	store := &fakeStateStore{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(flow.NewCategoryManageTargetFlow(fakeTargetLister{all: targetCatalog()}))
	return engine, store
}

func targetSeed(count string, withSuggestion bool) conversation.Data {
	seed := conversation.Data{
		conversation.KeySourceSubcategoryID: "7",
		conversation.KeySourceCategory:      "Comida",
		conversation.KeySourceSubcategory:   "Delivery",
		conversation.KeyMovementCount:       count,
	}
	if withSuggestion {
		seed[conversation.KeySuggestedSubcategoryID] = "3"
		seed[conversation.KeySuggestedCategory] = "Alimentos"
		seed[conversation.KeySuggestedSubcategory] = "Delivery"
	}
	return seed
}

func TestCategoryManageTargetFlow_BuildsWithoutPanic(t *testing.T) {
	fl := flow.NewCategoryManageTargetFlow(fakeTargetLister{})
	if fl.InitialStep != flow.StepSuggestTarget {
		t.Errorf("InitialStep = %q, want %q", fl.InitialStep, flow.StepSuggestTarget)
	}
}

func TestTargetFlow_ZeroCount_SkipsToConfirm(t *testing.T) {
	engine, store := newTargetEngine()
	prompt, err := engine.StartWithData(1, flow.CategoryManageTargetFlowName, targetSeed("0", false))
	if err != nil {
		t.Fatalf("StartWithData: %v", err)
	}
	if store.stepName != flow.StepConfirmCategoryManage {
		t.Fatalf("stepName = %q, want %q", store.stepName, flow.StepConfirmCategoryManage)
	}
	if prompt.Text != flow.MsgCategoryManageConfirmDelete("Comida › Delivery") {
		t.Errorf("Text = %q, want el confirm de borrado", prompt.Text)
	}
	for _, b := range prompt.Buttons {
		if b.Data == flow.OptionBack {
			t.Error("el confirm con conteo 0 no debería ofrecer Atrás")
		}
	}
}

func TestTargetFlow_NoSuggestion_SkipsToCategoryPicker(t *testing.T) {
	engine, store := newTargetEngine()
	prompt, err := engine.StartWithData(1, flow.CategoryManageTargetFlowName, targetSeed("3", false))
	if err != nil {
		t.Fatalf("StartWithData: %v", err)
	}
	if store.stepName != flow.StepPickTargetCategory {
		t.Fatalf("stepName = %q, want %q", store.stepName, flow.StepPickTargetCategory)
	}
	for _, b := range prompt.Buttons {
		if b.Data == flow.OptionBack {
			t.Error("sin sugerencia previa, el picker de categoría no debería ofrecer Atrás")
		}
	}
}

func TestTargetFlow_WithSuggestion_ShowsSuggestStep(t *testing.T) {
	engine, store := newTargetEngine()
	prompt, err := engine.StartWithData(1, flow.CategoryManageTargetFlowName, targetSeed("3", true))
	if err != nil {
		t.Fatalf("StartWithData: %v", err)
	}
	if store.stepName != flow.StepSuggestTarget {
		t.Fatalf("stepName = %q, want %q", store.stepName, flow.StepSuggestTarget)
	}
	want := flow.MsgCategoryManageSuggest("Comida › Delivery", "3", "Alimentos › Delivery")
	if prompt.Text != want {
		t.Errorf("Text = %q, want %q", prompt.Text, want)
	}
	if len(prompt.Buttons) != 3 {
		t.Fatalf("len(Buttons) = %d, want 3", len(prompt.Buttons))
	}
	if prompt.Buttons[0].Data != flow.OptionAcceptSuggestion {
		t.Errorf("Buttons[0] = %q, want %q", prompt.Buttons[0].Data, flow.OptionAcceptSuggestion)
	}
}

func TestTargetFlow_AcceptSuggestion_SetsTargetAndGoesToConfirm(t *testing.T) {
	engine, store := newTargetEngine()
	engine.StartWithData(1, flow.CategoryManageTargetFlowName, targetSeed("3", true))

	if _, _, err := engine.Handle(1, conversation.Input{CallbackData: flow.OptionAcceptSuggestion}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if store.stepName != flow.StepConfirmCategoryManage {
		t.Fatalf("stepName = %q, want %q", store.stepName, flow.StepConfirmCategoryManage)
	}
	if store.data[conversation.KeyTargetSubcategoryID] != "3" {
		t.Errorf("target id = %v, want \"3\"", store.data[conversation.KeyTargetSubcategoryID])
	}
	if store.data[conversation.KeyTargetCategory] != "Alimentos" {
		t.Errorf("target category = %v, want Alimentos", store.data[conversation.KeyTargetCategory])
	}
	if store.data[conversation.KeyTargetOrigin] != flow.TargetOriginSuggested {
		t.Errorf("target origin = %v, want %q", store.data[conversation.KeyTargetOrigin], flow.TargetOriginSuggested)
	}
}

func TestTargetFlow_ChooseOther_GoesToCategoryPickerWithBack(t *testing.T) {
	engine, store := newTargetEngine()
	engine.StartWithData(1, flow.CategoryManageTargetFlowName, targetSeed("3", true))

	result, _, err := engine.Handle(1, conversation.Input{CallbackData: flow.OptionChooseOther})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if store.stepName != flow.StepPickTargetCategory {
		t.Fatalf("stepName = %q, want %q", store.stepName, flow.StepPickTargetCategory)
	}
	found := false
	for _, b := range result.Prompt.Buttons {
		if b.Data == flow.OptionBack {
			found = true
		}
	}
	if !found {
		t.Error("con sugerencia previa, el picker debería ofrecer Atrás")
	}
}

func TestTargetFlow_SubcategoryPicker_ExcludesSource(t *testing.T) {
	engine, store := newTargetEngine()
	engine.StartWithData(1, flow.CategoryManageTargetFlowName, targetSeed("3", false))

	result, _, err := engine.Handle(1, conversation.Input{CallbackData: "Comida"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if store.stepName != flow.StepPickTargetSubcategory {
		t.Fatalf("stepName = %q, want %q", store.stepName, flow.StepPickTargetSubcategory)
	}
	for _, b := range result.Prompt.Buttons {
		if b.Data == "7" {
			t.Error("el picker ofreció la subcategoría de origen")
		}
	}
}

func TestTargetFlow_ManualPick_SetsTargetWithManualOrigin(t *testing.T) {
	engine, store := newTargetEngine()
	engine.StartWithData(1, flow.CategoryManageTargetFlowName, targetSeed("3", false))
	engine.Handle(1, conversation.Input{CallbackData: "Alimentos"})

	if _, _, err := engine.Handle(1, conversation.Input{CallbackData: "3"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if store.stepName != flow.StepConfirmCategoryManage {
		t.Fatalf("stepName = %q, want %q", store.stepName, flow.StepConfirmCategoryManage)
	}
	if store.data[conversation.KeyTargetSubcategoryID] != "3" {
		t.Errorf("target id = %v, want \"3\"", store.data[conversation.KeyTargetSubcategoryID])
	}
	if store.data[conversation.KeyTargetOrigin] != flow.TargetOriginManual {
		t.Errorf("target origin = %v, want %q", store.data[conversation.KeyTargetOrigin], flow.TargetOriginManual)
	}
}

func TestTargetFlow_BackFromConfirm_ClearsTarget(t *testing.T) {
	engine, store := newTargetEngine()
	engine.StartWithData(1, flow.CategoryManageTargetFlowName, targetSeed("3", true))
	engine.Handle(1, conversation.Input{CallbackData: flow.OptionAcceptSuggestion})

	if store.data[conversation.KeyTargetSubcategoryID] != "3" {
		t.Fatalf("precondición: el destino debería estar seteado, got %v", store.data[conversation.KeyTargetSubcategoryID])
	}
	if _, _, err := engine.Handle(1, conversation.Input{CallbackData: flow.OptionBack}); err != nil {
		t.Fatalf("Handle back: %v", err)
	}
	if store.stepName != flow.StepSuggestTarget {
		t.Errorf("stepName = %q, want %q", store.stepName, flow.StepSuggestTarget)
	}
	if got := conversation.StringOrEmpty(store.data[conversation.KeyTargetSubcategoryID]); got != "" {
		t.Errorf("target id = %q tras Atrás, want vacío", got)
	}
	if got := conversation.StringOrEmpty(store.data[conversation.KeyTargetOrigin]); got != "" {
		t.Errorf("target origin = %q tras Atrás, want vacío", got)
	}
}

func TestTargetFlow_ChooseOtherAfterAccept_ClearsTarget(t *testing.T) {
	engine, store := newTargetEngine()
	engine.StartWithData(1, flow.CategoryManageTargetFlowName, targetSeed("3", true))
	engine.Handle(1, conversation.Input{CallbackData: flow.OptionAcceptSuggestion})
	engine.Handle(1, conversation.Input{CallbackData: flow.OptionBack})

	if _, _, err := engine.Handle(1, conversation.Input{CallbackData: flow.OptionChooseOther}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got := conversation.StringOrEmpty(store.data[conversation.KeyTargetSubcategoryID]); got != "" {
		t.Errorf("target id = %q, want vacío tras elegir otra", got)
	}
}

func TestTargetFlow_ConfirmMergeCopyShowsCountAndBothNames(t *testing.T) {
	engine, _ := newTargetEngine()
	engine.StartWithData(1, flow.CategoryManageTargetFlowName, targetSeed("3", true))

	result, _, err := engine.Handle(1, conversation.Input{CallbackData: flow.OptionAcceptSuggestion})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	want := flow.MsgCategoryManageConfirmMerge("3", "Comida › Delivery", "Alimentos › Delivery")
	if result.Prompt.Text != want {
		t.Errorf("Text = %q, want %q", result.Prompt.Text, want)
	}
}

func TestTargetFlow_Confirm_FinishesConfirmed(t *testing.T) {
	engine, _ := newTargetEngine()
	engine.StartWithData(1, flow.CategoryManageTargetFlowName, targetSeed("3", true))
	engine.Handle(1, conversation.Input{CallbackData: flow.OptionAcceptSuggestion})

	result, _, err := engine.Handle(1, conversation.Input{CallbackData: flow.OptionConfirm})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !result.Finished {
		t.Fatal("confirmar debería terminar el flujo")
	}
	if !conversation.Flag(result.Data, conversation.KeyConfirmed) {
		t.Error("confirmar debería marcar conversation.KeyConfirmed")
	}
	if conversation.Flag(result.Data, conversation.KeyCancelled) {
		t.Error("confirmar no debería marcar cancelado")
	}
}

func TestTargetFlow_CancelAtConfirm_MarksCancelledNotConfirmed(t *testing.T) {
	engine, _ := newTargetEngine()
	engine.StartWithData(1, flow.CategoryManageTargetFlowName, targetSeed("0", false))

	result, _, err := engine.Handle(1, conversation.Input{CallbackData: flow.OptionCancel})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !conversation.Flag(result.Data, conversation.KeyCancelled) {
		t.Error("cancelar debería marcar conversation.KeyCancelled")
	}
	if conversation.Flag(result.Data, conversation.KeyConfirmed) {
		t.Error("cancelar no debería marcar confirmado")
	}
}

func TestTargetFlow_CancelAtSuggest_MarksCancelled(t *testing.T) {
	engine, _ := newTargetEngine()
	engine.StartWithData(1, flow.CategoryManageTargetFlowName, targetSeed("3", true))

	result, _, err := engine.Handle(1, conversation.Input{CallbackData: flow.OptionCancel})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !conversation.Flag(result.Data, conversation.KeyCancelled) {
		t.Error("cancelar en la sugerencia debería marcar conversation.KeyCancelled")
	}
}

func TestTargetFlow_BackFromSubcategoryPicker_ReturnsToCategoryPicker(t *testing.T) {
	engine, store := newTargetEngine()
	engine.StartWithData(1, flow.CategoryManageTargetFlowName, targetSeed("3", false))
	engine.Handle(1, conversation.Input{CallbackData: "Alimentos"})

	if _, _, err := engine.Handle(1, conversation.Input{CallbackData: flow.OptionBack}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if store.stepName != flow.StepPickTargetCategory {
		t.Errorf("stepName = %q, want %q", store.stepName, flow.StepPickTargetCategory)
	}
}

func TestTargetFlow_CategoryPicker_ExcludesReserved(t *testing.T) {
	engine, _ := newTargetEngine()
	prompt, err := engine.StartWithData(1, flow.CategoryManageTargetFlowName, targetSeed("3", false))
	if err != nil {
		t.Fatalf("StartWithData: %v", err)
	}

	for _, b := range prompt.Buttons {
		if b.Data == "Sistema" || b.Data == "PENDING_REVIEW" {
			t.Errorf("el picker ofreció una reservada: %q", b.Data)
		}
	}
}

func TestProceedToCategoryTarget_ZeroCountSkipsOrchestrator(t *testing.T) {
	engine, store := newTargetEngine()
	orch := &fakeCategoryOrchestrator{}
	c := &controller{
		engine:        engine,
		orchestrator:  orch,
		subcategories: &fakeSubcategoryRepoFull{},
		movements:     &fakeMovementRepoFull{countBySubcategory: 0},
	}

	data := conversation.Data{
		conversation.UserIDKey:              uint64(1),
		conversation.KeySourceSubcategoryID: "7",
		conversation.KeySourceCategory:      "Comida",
		conversation.KeySourceSubcategory:   "Delivery",
	}
	if err := c.proceedToCategoryTarget(context.Background(), &messenger.FakeChat{}, data); err != nil {
		t.Fatalf("proceedToCategoryTarget: %v", err)
	}
	if orch.calls != 0 {
		t.Errorf("llamó al orquestador %d veces con conteo 0, want 0", orch.calls)
	}
	if store.data[conversation.KeyMovementCount] != "0" {
		t.Errorf("movement_count = %v, want \"0\"", store.data[conversation.KeyMovementCount])
	}
	if store.stepName != flow.StepConfirmCategoryManage {
		t.Errorf("stepName = %q, want %q", store.stepName, flow.StepConfirmCategoryManage)
	}
}

func TestProceedToCategoryTarget_PositiveCountSeedsSuggestion(t *testing.T) {
	all := targetCatalog()
	engine, store := newTargetEngine()
	orch := &fakeCategoryOrchestrator{
		match: &orchestrator.CategoryMatch{Category: "Alimentos", Subcategory: "Delivery"},
	}
	c := &controller{
		engine:       engine,
		orchestrator: orch,
		subcategories: &fakeSubcategoryRepoFull{
			all:              all,
			byCategoryAndSub: map[string]*subcategory.Subcategory{"Alimentos|Delivery": &all[1]},
		},
		movements: &fakeMovementRepoFull{countBySubcategory: 3},
	}

	data := conversation.Data{
		conversation.UserIDKey:              uint64(1),
		conversation.KeySourceSubcategoryID: "7",
		conversation.KeySourceCategory:      "Comida",
		conversation.KeySourceSubcategory:   "Delivery",
	}
	if err := c.proceedToCategoryTarget(context.Background(), &messenger.FakeChat{}, data); err != nil {
		t.Fatalf("proceedToCategoryTarget: %v", err)
	}
	if orch.calls != 1 {
		t.Errorf("llamó al orquestador %d veces, want 1", orch.calls)
	}
	if store.data[conversation.KeyMovementCount] != "3" {
		t.Errorf("movement_count = %v, want \"3\"", store.data[conversation.KeyMovementCount])
	}
	if store.data[conversation.KeySuggestedSubcategoryID] != "3" {
		t.Errorf("suggested id = %v, want \"3\"", store.data[conversation.KeySuggestedSubcategoryID])
	}
	if store.stepName != flow.StepSuggestTarget {
		t.Errorf("stepName = %q, want %q", store.stepName, flow.StepSuggestTarget)
	}
}

func TestProceedToCategoryTarget_OrchestratorErrorStillStartsFlow(t *testing.T) {
	engine, store := newTargetEngine()
	c := &controller{
		engine:        engine,
		orchestrator:  &fakeCategoryOrchestrator{err: errFake},
		subcategories: &fakeSubcategoryRepoFull{all: targetCatalog()},
		movements:     &fakeMovementRepoFull{countBySubcategory: 3},
	}

	data := conversation.Data{
		conversation.UserIDKey:              uint64(1),
		conversation.KeySourceSubcategoryID: "7",
		conversation.KeySourceCategory:      "Comida",
		conversation.KeySourceSubcategory:   "Delivery",
	}
	if err := c.proceedToCategoryTarget(context.Background(), &messenger.FakeChat{}, data); err != nil {
		t.Fatalf("un fallo del LLM no debería romper el flujo: %v", err)
	}
	if store.stepName != flow.StepPickTargetCategory {
		t.Errorf("stepName = %q, want %q (picker manual)", store.stepName, flow.StepPickTargetCategory)
	}
}

func TestProceedToCategoryTarget_CountErrorReturnsError(t *testing.T) {
	engine, store := newTargetEngine()
	c := &controller{
		engine:        engine,
		orchestrator:  &fakeCategoryOrchestrator{},
		subcategories: &fakeSubcategoryRepoFull{},
		movements:     &fakeMovementRepoFull{countErr: errFake},
	}

	data := conversation.Data{
		conversation.UserIDKey:              uint64(1),
		conversation.KeySourceSubcategoryID: "7",
	}
	if err := c.proceedToCategoryTarget(context.Background(), &messenger.FakeChat{}, data); err == nil {
		t.Error("un error al contar debería propagarse")
	}
	if store.stepName != "" {
		t.Errorf("arrancó el flujo (%q) pese al error de conteo", store.stepName)
	}
}

func TestTargetFlow_BackFromConfirm_ManualPath_KeepsCategoryAndListsOptions(t *testing.T) {
	engine, store := newTargetEngine()
	engine.StartWithData(1, flow.CategoryManageTargetFlowName, targetSeed("3", false))
	engine.Handle(1, conversation.Input{CallbackData: "Alimentos"})
	engine.Handle(1, conversation.Input{CallbackData: "3"})

	if store.stepName != flow.StepConfirmCategoryManage {
		t.Fatalf("precondición: stepName = %q, want %q", store.stepName, flow.StepConfirmCategoryManage)
	}

	result, _, err := engine.Handle(1, conversation.Input{CallbackData: flow.OptionBack})
	if err != nil {
		t.Fatalf("Handle back: %v", err)
	}
	if store.stepName != flow.StepPickTargetSubcategory {
		t.Fatalf("stepName = %q, want %q", store.stepName, flow.StepPickTargetSubcategory)
	}
	if got := conversation.StringOrEmpty(store.data[conversation.KeyTargetCategory]); got != "Alimentos" {
		t.Errorf("conversation.KeyTargetCategory = %q tras Atrás, want %q conservada", got, "Alimentos")
	}
	if got := conversation.StringOrEmpty(store.data[conversation.KeyTargetSubcategoryID]); got != "" {
		t.Errorf("conversation.KeyTargetSubcategoryID = %q tras Atrás, want vacío", got)
	}

	picks := 0
	for _, b := range result.Prompt.Buttons {
		if b.Data != flow.OptionBack && b.Data != flow.OptionCancel {
			picks++
		}
	}
	if picks == 0 {
		t.Error("el picker de subcategoría quedó sin opciones elegibles tras volver del confirm")
	}
}

type raceTargetLister struct {
	calls  *int
	before []subcategory.Subcategory
	after  []subcategory.Subcategory
	missAt int
}

func (r raceTargetLister) FindAllForUser(uint64) ([]subcategory.Subcategory, error) {
	*r.calls++
	if *r.calls >= r.missAt {
		return r.after, nil
	}
	return r.before, nil
}

func (r raceTargetLister) DistinctCategoriesForUser(uint64) ([]string, error) {
	return []string{"Alimentos"}, nil
}

func (r raceTargetLister) IconForCategory(uint64, string) string { return flow.DefaultCategoryIcon }

func TestTargetFlow_TargetRowDisappears_NoPartialTarget(t *testing.T) {
	full := targetCatalog()
	calls := 0
	lister := raceTargetLister{
		calls:  &calls,
		before: full,
		after:  []subcategory.Subcategory{ownedSub(7, "Comida", "Delivery", "🍕")},
		missAt: 2,
	}

	store := &fakeStateStore{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(flow.NewCategoryManageTargetFlow(lister))

	engine.StartWithData(1, flow.CategoryManageTargetFlowName, targetSeed("3", false))
	engine.Handle(1, conversation.Input{CallbackData: "Alimentos"})
	calls = 0

	engine.Handle(1, conversation.Input{CallbackData: "3"})

	id := conversation.StringOrEmpty(store.data[conversation.KeyTargetSubcategoryID])
	name := conversation.StringOrEmpty(store.data[conversation.KeyTargetSubcategory])
	if id != "" && name == "" {
		t.Errorf("destino a medias: ID=%q sin nombre — el confirm mostraría «Alimentos › »", id)
	}
}

type fakeCategoryOrchestrator struct {
	movementOrchestrator
	match       *orchestrator.CategoryMatch
	proposal    *orchestrator.CategoryProposal
	err         error
	calls       int
	gotText     string
	gotTaxonomy []orchestrator.TaxonomyEntry
}

func (f *fakeCategoryOrchestrator) ClassifyCategoryCreate(_ context.Context, text string, taxonomy []orchestrator.TaxonomyEntry) (orchestrator.CategoryCreateResult, error) {
	f.calls++
	f.gotText, f.gotTaxonomy = text, taxonomy
	if f.err != nil {
		return orchestrator.CategoryCreateResult{}, f.err
	}
	return orchestrator.CategoryCreateResult{Match: f.match, Proposal: f.proposal}, nil
}
