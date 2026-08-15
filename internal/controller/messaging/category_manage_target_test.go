package messaging

import (
	"context"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
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

// El grafo del flujo 2 tiene que validar: si algún NextStep apuntara a un step
// inexistente, NewFlow haría panic acá. Es la red del picker parametrizado.
func TestCategoryManageTargetFlow_BuildsWithoutPanic(t *testing.T) {
	fl := flow.NewCategoryManageTargetFlow(fakeTargetLister{})
	if fl.InitialStep != flow.StepSuggestTarget {
		t.Errorf("InitialStep = %q, want %q", fl.InitialStep, flow.StepSuggestTarget)
	}
}

// Conteo 0 → ni sugerencia ni pickers: directo al confirm de borrado.
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
	// sin Atrás: no hubo ninguna elección que rehacer
	for _, b := range prompt.Buttons {
		if b.Data == flow.OptionBack {
			t.Error("el confirm con conteo 0 no debería ofrecer Atrás")
		}
	}
}

// Con movimientos y sin sugerencia, arranca directo en el picker manual.
func TestTargetFlow_NoSuggestion_SkipsToCategoryPicker(t *testing.T) {
	engine, store := newTargetEngine()
	prompt, err := engine.StartWithData(1, flow.CategoryManageTargetFlowName, targetSeed("3", false))
	if err != nil {
		t.Fatalf("StartWithData: %v", err)
	}
	if store.stepName != flow.StepPickTargetCategory {
		t.Fatalf("stepName = %q, want %q", store.stepName, flow.StepPickTargetCategory)
	}
	// sin sugerencia, este es el primer step real: no lleva Atrás
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
	// hubo sugerencia, así que se puede volver a ella
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

// El picker de subcategoría destino nunca puede ofrecer la fila de origen.
func TestTargetFlow_SubcategoryPicker_ExcludesSource(t *testing.T) {
	engine, store := newTargetEngine()
	engine.StartWithData(1, flow.CategoryManageTargetFlowName, targetSeed("3", false))

	// elegir la categoría "Comida", que es la del origen
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

// EL test del destino stale: aceptar la sugerencia, volver, y elegir otra ruta
// no puede dejar el destino viejo colgado.
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

// Las reservadas nunca aparecen como destino.
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
		conversation.UserIDKey:              uint64(1),
		conversation.KeySourceSubcategoryID: "7",
		conversation.KeySourceCategory:      "Comida",
		conversation.KeySourceSubcategory:   "Delivery",
	}
	if err := c.proceedToCategoryTarget(context.Background(), nil, 100, data); err != nil {
		t.Fatalf("proceedToCategoryTarget: %v", err)
	}
	if orch.calls != 0 {
		t.Errorf("llamó al orquestador %d veces con conteo 0, want 0", orch.calls)
	}
	if store.data[conversation.KeyMovementCount] != "0" {
		t.Errorf("movement_count = %v, want \"0\"", store.data[conversation.KeyMovementCount])
	}
	// y con conteo 0 el flujo 2 tiene que haber saltado directo al confirm
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
	if err := c.proceedToCategoryTarget(context.Background(), nil, 100, data); err != nil {
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
		conversation.UserIDKey:              uint64(1),
		conversation.KeySourceSubcategoryID: "7",
		conversation.KeySourceCategory:      "Comida",
		conversation.KeySourceSubcategory:   "Delivery",
	}
	if err := c.proceedToCategoryTarget(context.Background(), nil, 100, data); err != nil {
		t.Fatalf("un fallo del LLM no debería romper el flujo: %v", err)
	}
	if store.stepName != flow.StepPickTargetCategory {
		t.Errorf("stepName = %q, want %q (picker manual)", store.stepName, flow.StepPickTargetCategory)
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
		conversation.UserIDKey:              uint64(1),
		conversation.KeySourceSubcategoryID: "7",
	}
	if err := c.proceedToCategoryTarget(context.Background(), nil, 100, data); err == nil {
		t.Error("un error al contar debería propagarse")
	}
	if store.stepName != "" {
		t.Errorf("arrancó el flujo (%q) pese al error de conteo", store.stepName)
	}
}

// Volver con Atrás desde el confirm por el camino MANUAL tiene que dejar al
// usuario en el picker de subcategoría con opciones reales. Si la categoría
// destino se borrara junto con la subcategoría, ese picker no tendría nada que
// listar (filtra por categoría) y el usuario quedaría en un callejón.
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

	// lo que realmente importa: el picker tiene algo para elegir
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

// raceTargetLister es el equivalente de raceLister para el flujo 2: devuelve el
// catálogo completo en las primeras consultas y uno recortado a partir de
// missAt, simulando un Reload() concurrente en el medio de la elección.
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

// Si la fila destino desaparece justo antes de que OnChoice busque su nombre,
// NO puede quedar un destino con ID y sin nombre: el confirm diría
// «Alimentos › » y el usuario no podría verificar qué está por confirmar, en
// una operación que mueve movimientos y borra una fila.
func TestTargetFlow_TargetRowDisappears_NoPartialTarget(t *testing.T) {
	full := targetCatalog()
	calls := 0
	lister := raceTargetLister{
		calls:  &calls,
		before: full,
		after:  []subcategory.Subcategory{ownedSub(7, "Comida", "Delivery", "🍕")}, // sin la 3
		missAt: 2,                                                                 // dentro de Handle("3"): 1=validación de la opción, 2=lookup del nombre
	}

	store := &fakeStateStore{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(flow.NewCategoryManageTargetFlow(lister))

	engine.StartWithData(1, flow.CategoryManageTargetFlowName, targetSeed("3", false))
	engine.Handle(1, conversation.Input{CallbackData: "Alimentos"})
	// El prompt del paso de subcategoría ya se renderizó dentro del Handle
	// anterior, así que el contador arranca de cero recién acá.
	calls = 0

	engine.Handle(1, conversation.Input{CallbackData: "3"})

	id := conversation.StringOrEmpty(store.data[conversation.KeyTargetSubcategoryID])
	name := conversation.StringOrEmpty(store.data[conversation.KeyTargetSubcategory])
	if id != "" && name == "" {
		t.Errorf("destino a medias: ID=%q sin nombre — el confirm mostraría «Alimentos › »", id)
	}
}
