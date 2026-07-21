package messaging

import (
	"context"
	"testing"

	"lopiibot.com/internal/conversation"
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
			return subcategoryIcon(s)
		}
	}
	return defaultCategoryIcon
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
	engine.Register(NewCategoryManageTargetFlow(fakeTargetLister{all: targetCatalog()}))
	return engine, store
}

func targetSeed(count string, withSuggestion bool) conversation.Data {
	seed := conversation.Data{
		keySourceSubcategoryID: "7",
		keySourceCategory:      "Comida",
		keySourceSubcategory:   "Delivery",
		keyMovementCount:       count,
	}
	if withSuggestion {
		seed[keySuggestedSubcategoryID] = "3"
		seed[keySuggestedCategory] = "Alimentos"
		seed[keySuggestedSubcategory] = "Delivery"
	}
	return seed
}

// El grafo del flujo 2 tiene que validar: si algún NextStep apuntara a un step
// inexistente, NewFlow haría panic acá. Es la red del picker parametrizado.
func TestCategoryManageTargetFlow_BuildsWithoutPanic(t *testing.T) {
	flow := NewCategoryManageTargetFlow(fakeTargetLister{})
	if flow.InitialStep != stepSuggestTarget {
		t.Errorf("InitialStep = %q, want %q", flow.InitialStep, stepSuggestTarget)
	}
}

// Conteo 0 → ni sugerencia ni pickers: directo al confirm de borrado.
func TestTargetFlow_ZeroCount_SkipsToConfirm(t *testing.T) {
	engine, store := newTargetEngine()
	prompt, err := engine.StartWithData(1, categoryManageTargetFlowName, targetSeed("0", false))
	if err != nil {
		t.Fatalf("StartWithData: %v", err)
	}
	if store.stepName != stepConfirmCategoryManage {
		t.Fatalf("stepName = %q, want %q", store.stepName, stepConfirmCategoryManage)
	}
	if prompt.Text != msgCategoryManageConfirmDelete("Comida › Delivery") {
		t.Errorf("Text = %q, want el confirm de borrado", prompt.Text)
	}
	// sin Atrás: no hubo ninguna elección que rehacer
	for _, b := range prompt.Buttons {
		if b.Data == optionBack {
			t.Error("el confirm con conteo 0 no debería ofrecer Atrás")
		}
	}
}

// Con movimientos y sin sugerencia, arranca directo en el picker manual.
func TestTargetFlow_NoSuggestion_SkipsToCategoryPicker(t *testing.T) {
	engine, store := newTargetEngine()
	prompt, err := engine.StartWithData(1, categoryManageTargetFlowName, targetSeed("3", false))
	if err != nil {
		t.Fatalf("StartWithData: %v", err)
	}
	if store.stepName != stepPickTargetCategory {
		t.Fatalf("stepName = %q, want %q", store.stepName, stepPickTargetCategory)
	}
	// sin sugerencia, este es el primer step real: no lleva Atrás
	for _, b := range prompt.Buttons {
		if b.Data == optionBack {
			t.Error("sin sugerencia previa, el picker de categoría no debería ofrecer Atrás")
		}
	}
}

func TestTargetFlow_WithSuggestion_ShowsSuggestStep(t *testing.T) {
	engine, store := newTargetEngine()
	prompt, err := engine.StartWithData(1, categoryManageTargetFlowName, targetSeed("3", true))
	if err != nil {
		t.Fatalf("StartWithData: %v", err)
	}
	if store.stepName != stepSuggestTarget {
		t.Fatalf("stepName = %q, want %q", store.stepName, stepSuggestTarget)
	}
	want := msgCategoryManageSuggest("Comida › Delivery", "3", "Alimentos › Delivery")
	if prompt.Text != want {
		t.Errorf("Text = %q, want %q", prompt.Text, want)
	}
	if len(prompt.Buttons) != 3 {
		t.Fatalf("len(Buttons) = %d, want 3", len(prompt.Buttons))
	}
	if prompt.Buttons[0].Data != optionAcceptSuggestion {
		t.Errorf("Buttons[0] = %q, want %q", prompt.Buttons[0].Data, optionAcceptSuggestion)
	}
}

func TestTargetFlow_AcceptSuggestion_SetsTargetAndGoesToConfirm(t *testing.T) {
	engine, store := newTargetEngine()
	engine.StartWithData(1, categoryManageTargetFlowName, targetSeed("3", true))

	if _, _, err := engine.Handle(1, conversation.Input{CallbackData: optionAcceptSuggestion}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if store.stepName != stepConfirmCategoryManage {
		t.Fatalf("stepName = %q, want %q", store.stepName, stepConfirmCategoryManage)
	}
	if store.data[keyTargetSubcategoryID] != "3" {
		t.Errorf("target id = %v, want \"3\"", store.data[keyTargetSubcategoryID])
	}
	if store.data[keyTargetCategory] != "Alimentos" {
		t.Errorf("target category = %v, want Alimentos", store.data[keyTargetCategory])
	}
	if store.data[keyTargetOrigin] != targetOriginSuggested {
		t.Errorf("target origin = %v, want %q", store.data[keyTargetOrigin], targetOriginSuggested)
	}
}

func TestTargetFlow_ChooseOther_GoesToCategoryPickerWithBack(t *testing.T) {
	engine, store := newTargetEngine()
	engine.StartWithData(1, categoryManageTargetFlowName, targetSeed("3", true))

	result, _, err := engine.Handle(1, conversation.Input{CallbackData: optionChooseOther})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if store.stepName != stepPickTargetCategory {
		t.Fatalf("stepName = %q, want %q", store.stepName, stepPickTargetCategory)
	}
	// hubo sugerencia, así que se puede volver a ella
	found := false
	for _, b := range result.Prompt.Buttons {
		if b.Data == optionBack {
			found = true
		}
	}
	if !found {
		t.Error("con sugerencia previa, el picker debería ofrecer Atrás")
	}
}

// El picker de subcategoría destino nunca puede ofrecer la fila de origen.
func TestTargetFlow_SubcategoryPicker_ExcludesSource(t *testing.T) {
	engine, store := newTargetEngine()
	engine.StartWithData(1, categoryManageTargetFlowName, targetSeed("3", false))

	// elegir la categoría "Comida", que es la del origen
	result, _, err := engine.Handle(1, conversation.Input{CallbackData: "Comida"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if store.stepName != stepPickTargetSubcategory {
		t.Fatalf("stepName = %q, want %q", store.stepName, stepPickTargetSubcategory)
	}
	for _, b := range result.Prompt.Buttons {
		if b.Data == "7" {
			t.Error("el picker ofreció la subcategoría de origen")
		}
	}
}

func TestTargetFlow_ManualPick_SetsTargetWithManualOrigin(t *testing.T) {
	engine, store := newTargetEngine()
	engine.StartWithData(1, categoryManageTargetFlowName, targetSeed("3", false))
	engine.Handle(1, conversation.Input{CallbackData: "Alimentos"})

	if _, _, err := engine.Handle(1, conversation.Input{CallbackData: "3"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if store.stepName != stepConfirmCategoryManage {
		t.Fatalf("stepName = %q, want %q", store.stepName, stepConfirmCategoryManage)
	}
	if store.data[keyTargetSubcategoryID] != "3" {
		t.Errorf("target id = %v, want \"3\"", store.data[keyTargetSubcategoryID])
	}
	if store.data[keyTargetOrigin] != targetOriginManual {
		t.Errorf("target origin = %v, want %q", store.data[keyTargetOrigin], targetOriginManual)
	}
}

// EL test del destino stale: aceptar la sugerencia, volver, y elegir otra ruta
// no puede dejar el destino viejo colgado.
func TestTargetFlow_BackFromConfirm_ClearsTarget(t *testing.T) {
	engine, store := newTargetEngine()
	engine.StartWithData(1, categoryManageTargetFlowName, targetSeed("3", true))
	engine.Handle(1, conversation.Input{CallbackData: optionAcceptSuggestion})

	if store.data[keyTargetSubcategoryID] != "3" {
		t.Fatalf("precondición: el destino debería estar seteado, got %v", store.data[keyTargetSubcategoryID])
	}
	if _, _, err := engine.Handle(1, conversation.Input{CallbackData: optionBack}); err != nil {
		t.Fatalf("Handle back: %v", err)
	}
	if store.stepName != stepSuggestTarget {
		t.Errorf("stepName = %q, want %q", store.stepName, stepSuggestTarget)
	}
	if got := stringOrEmpty(store.data[keyTargetSubcategoryID]); got != "" {
		t.Errorf("target id = %q tras Atrás, want vacío", got)
	}
	if got := stringOrEmpty(store.data[keyTargetOrigin]); got != "" {
		t.Errorf("target origin = %q tras Atrás, want vacío", got)
	}
}

func TestTargetFlow_ChooseOtherAfterAccept_ClearsTarget(t *testing.T) {
	engine, store := newTargetEngine()
	engine.StartWithData(1, categoryManageTargetFlowName, targetSeed("3", true))
	engine.Handle(1, conversation.Input{CallbackData: optionAcceptSuggestion})
	engine.Handle(1, conversation.Input{CallbackData: optionBack})

	if _, _, err := engine.Handle(1, conversation.Input{CallbackData: optionChooseOther}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got := stringOrEmpty(store.data[keyTargetSubcategoryID]); got != "" {
		t.Errorf("target id = %q, want vacío tras elegir otra", got)
	}
}

func TestTargetFlow_ConfirmMergeCopyShowsCountAndBothNames(t *testing.T) {
	engine, _ := newTargetEngine()
	engine.StartWithData(1, categoryManageTargetFlowName, targetSeed("3", true))

	result, _, err := engine.Handle(1, conversation.Input{CallbackData: optionAcceptSuggestion})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	want := msgCategoryManageConfirmMerge("3", "Comida › Delivery", "Alimentos › Delivery")
	if result.Prompt.Text != want {
		t.Errorf("Text = %q, want %q", result.Prompt.Text, want)
	}
}

func TestTargetFlow_Confirm_FinishesConfirmed(t *testing.T) {
	engine, _ := newTargetEngine()
	engine.StartWithData(1, categoryManageTargetFlowName, targetSeed("3", true))
	engine.Handle(1, conversation.Input{CallbackData: optionAcceptSuggestion})

	result, _, err := engine.Handle(1, conversation.Input{CallbackData: optionConfirm})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !result.Finished {
		t.Fatal("confirmar debería terminar el flujo")
	}
	if !flag(result.Data, keyConfirmed) {
		t.Error("confirmar debería marcar keyConfirmed")
	}
	if flag(result.Data, keyCancelled) {
		t.Error("confirmar no debería marcar cancelado")
	}
}

func TestTargetFlow_CancelAtConfirm_MarksCancelledNotConfirmed(t *testing.T) {
	engine, _ := newTargetEngine()
	engine.StartWithData(1, categoryManageTargetFlowName, targetSeed("0", false))

	result, _, err := engine.Handle(1, conversation.Input{CallbackData: optionCancel})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !flag(result.Data, keyCancelled) {
		t.Error("cancelar debería marcar keyCancelled")
	}
	if flag(result.Data, keyConfirmed) {
		t.Error("cancelar no debería marcar confirmado")
	}
}

func TestTargetFlow_CancelAtSuggest_MarksCancelled(t *testing.T) {
	engine, _ := newTargetEngine()
	engine.StartWithData(1, categoryManageTargetFlowName, targetSeed("3", true))

	result, _, err := engine.Handle(1, conversation.Input{CallbackData: optionCancel})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !flag(result.Data, keyCancelled) {
		t.Error("cancelar en la sugerencia debería marcar keyCancelled")
	}
}

func TestTargetFlow_BackFromSubcategoryPicker_ReturnsToCategoryPicker(t *testing.T) {
	engine, store := newTargetEngine()
	engine.StartWithData(1, categoryManageTargetFlowName, targetSeed("3", false))
	engine.Handle(1, conversation.Input{CallbackData: "Alimentos"})

	if _, _, err := engine.Handle(1, conversation.Input{CallbackData: optionBack}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if store.stepName != stepPickTargetCategory {
		t.Errorf("stepName = %q, want %q", store.stepName, stepPickTargetCategory)
	}
}

// Las reservadas nunca aparecen como destino.
func TestTargetFlow_CategoryPicker_ExcludesReserved(t *testing.T) {
	engine, _ := newTargetEngine()
	prompt, err := engine.StartWithData(1, categoryManageTargetFlowName, targetSeed("3", false))
	if err != nil {
		t.Fatalf("StartWithData: %v", err)
	}

	for _, b := range prompt.Buttons {
		if b.Data == "Sistema" || b.Data == "PENDING_REVIEW" {
			t.Errorf("el picker ofreció una reservada: %q", b.Data)
		}
	}
}

// --- puente (Task 8), testeable recién ahora que el flujo 2 existe ---

// Conteo 0 → ni se le pregunta al LLM. Ahorra una llamada y latencia en el
// camino más común de "esta categoría está vacía, sacala".
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
		conversation.UserIDKey: uint64(1),
		keySourceSubcategoryID: "7",
		keySourceCategory:      "Comida",
		keySourceSubcategory:   "Delivery",
	}
	if err := c.proceedToCategoryTarget(context.Background(), nil, 100, data); err != nil {
		t.Fatalf("proceedToCategoryTarget: %v", err)
	}
	if orch.calls != 0 {
		t.Errorf("llamó al orquestador %d veces con conteo 0, want 0", orch.calls)
	}
	if store.data[keyMovementCount] != "0" {
		t.Errorf("movement_count = %v, want \"0\"", store.data[keyMovementCount])
	}
	// y con conteo 0 el flujo 2 tiene que haber saltado directo al confirm
	if store.stepName != stepConfirmCategoryManage {
		t.Errorf("stepName = %q, want %q", store.stepName, stepConfirmCategoryManage)
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
		conversation.UserIDKey: uint64(1),
		keySourceSubcategoryID: "7",
		keySourceCategory:      "Comida",
		keySourceSubcategory:   "Delivery",
	}
	if err := c.proceedToCategoryTarget(context.Background(), nil, 100, data); err != nil {
		t.Fatalf("proceedToCategoryTarget: %v", err)
	}
	if orch.calls != 1 {
		t.Errorf("llamó al orquestador %d veces, want 1", orch.calls)
	}
	if store.data[keyMovementCount] != "3" {
		t.Errorf("movement_count = %v, want \"3\"", store.data[keyMovementCount])
	}
	if store.data[keySuggestedSubcategoryID] != "3" {
		t.Errorf("suggested id = %v, want \"3\"", store.data[keySuggestedSubcategoryID])
	}
	if store.stepName != stepSuggestTarget {
		t.Errorf("stepName = %q, want %q", store.stepName, stepSuggestTarget)
	}
}

// Sin sugerencia utilizable, el flujo arranca igual — en el picker manual.
// La sugerencia es un atajo, nunca un bloqueo.
func TestProceedToCategoryTarget_OrchestratorErrorStillStartsFlow(t *testing.T) {
	engine, store := newTargetEngine()
	c := &controller{
		engine:        engine,
		orchestrator:  &fakeCategoryOrchestrator{err: errFake},
		subcategories: &fakeSubcategoryRepoFull{all: targetCatalog()},
		movements:     &fakeMovementRepoFull{countBySubcategory: 3},
	}

	data := conversation.Data{
		conversation.UserIDKey: uint64(1),
		keySourceSubcategoryID: "7",
		keySourceCategory:      "Comida",
		keySourceSubcategory:   "Delivery",
	}
	if err := c.proceedToCategoryTarget(context.Background(), nil, 100, data); err != nil {
		t.Fatalf("un fallo del LLM no debería romper el flujo: %v", err)
	}
	if store.stepName != stepPickTargetCategory {
		t.Errorf("stepName = %q, want %q (picker manual)", store.stepName, stepPickTargetCategory)
	}
}

// Un error al contar sí es fatal: sin el número no se puede mostrar un confirm
// honesto, y este flujo borra datos.
func TestProceedToCategoryTarget_CountErrorReturnsError(t *testing.T) {
	engine, store := newTargetEngine()
	c := &controller{
		engine:        engine,
		orchestrator:  &fakeCategoryOrchestrator{},
		subcategories: &fakeSubcategoryRepoFull{},
		movements:     &fakeMovementRepoFull{countErr: errFake},
	}

	data := conversation.Data{
		conversation.UserIDKey: uint64(1),
		keySourceSubcategoryID: "7",
	}
	if err := c.proceedToCategoryTarget(context.Background(), nil, 100, data); err == nil {
		t.Error("un error al contar debería propagarse")
	}
	if store.stepName != "" {
		t.Errorf("arrancó el flujo (%q) pese al error de conteo", store.stepName)
	}
}
