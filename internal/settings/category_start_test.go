package settings

import (
	"context"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/subcategory"
)

func newPickEngine(owned []subcategory.Subcategory) (*conversation.Engine, *fakeStateStore) {
	store := &fakeStateStore{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(flow.NewCategoryManagePickFlow(fakeOwnedLister{owned: owned}))
	return engine, store
}

// Un usuario sin categorías propias vería un picker con solo "Cancelar": un
// callejón. El guard tiene que cortar antes de arrancar el flujo.
func TestStartCategoryManage_NoOwnCategories_DoesNotStartFlow(t *testing.T) {
	engine, store := newPickEngine(nil)
	s := &testServices{engine: engine}

	if err := StartCategoryManage(context.Background(), s, &messenger.FakeChat{}, 1); err != nil {
		t.Fatalf("StartCategoryManage: %v", err)
	}
	if store.stepName != "" {
		t.Errorf("arrancó un flujo (%q) con cero categorías propias", store.stepName)
	}
}

func TestStartCategoryManage_WithOwnCategories_StartsPickFlow(t *testing.T) {
	owned := []subcategory.Subcategory{ownedSub(7, "Comida", "Delivery", "🍕")}
	engine, store := newPickEngine(owned)
	s := &testServices{engine: engine, owned: owned}

	if err := StartCategoryManage(context.Background(), s, &messenger.FakeChat{}, 1); err != nil {
		t.Fatalf("StartCategoryManage: %v", err)
	}
	if store.stepName != flow.StepPickSource {
		t.Errorf("stepName = %q, want %q", store.stepName, flow.StepPickSource)
	}
}

// Un error del repo no puede dejar al usuario sin respuesta ni arrancar un
// flujo a medias.
func TestStartCategoryManage_RepoError_DoesNotStartFlow(t *testing.T) {
	engine, store := newPickEngine(nil)
	s := &testServices{engine: engine, ownedErr: errFake}

	if err := StartCategoryManage(context.Background(), s, &messenger.FakeChat{}, 1); err == nil {
		t.Error("un error del repo debería propagarse")
	}
	if store.stepName != "" {
		t.Errorf("arrancó un flujo (%q) pese al error", store.stepName)
	}
}

// El guard tiene que RESOLVER la métrica: el bot entendió y respondió bien.
// Sin esto el evento queda pendiente y el sweeper lo marca "abandoned", que en
// las métricas de asertividad se lee como una falla del bot. Pasó de verdad:
// el primer "tengo categorías repetidas" en producción quedó como abandoned.
func TestStartCategoryManage_NoOwnCategories_ResolvesMetric(t *testing.T) {
	engine, _ := newPickEngine(nil)
	s := &testServices{engine: engine}

	if err := StartCategoryManage(context.Background(), s, &messenger.FakeChat{}, 1); err != nil {
		t.Fatalf("StartCategoryManage: %v", err)
	}
	if len(s.resolved) != 1 || s.resolved[0] != OutcomeCategoryManageNoOwn {
		t.Errorf("resolved = %v, want [%s] (si queda sin resolver, el sweeper lo marca abandoned)",
			s.resolved, OutcomeCategoryManageNoOwn)
	}
}

// Un 429 en la clasificación NO puede caer al wizard: el usuario pagaría las 7
// preguntas por un problema de cupo que ya quedó encolado y se resuelve solo.
// El invariante vive en StartSubcategorySetup —HandleGroqError ANTES del
// fallback— y este test es su única red: el que lo cuidaba
// (TestCreateCategory_RateLimited_EnqueuesAndSkipsWizard) se fue con
// pending_jobs_test.go cuando el split borró ese archivo.
func TestStartSubcategorySetup_RateLimited_SkipsWizard(t *testing.T) {
	s := &testServices{
		engine:      conversation.NewEngine(&fakeStateStore{}, func(string) string { return "algo" }),
		classifyErr: errFake,
		groqHandled: true, // el 429 quedó encolado
	}

	err := StartSubcategorySetup(context.Background(), s, &messenger.FakeChat{}, 1, "creá gastos de regalos")

	// El orden importa: si el guard se rompe, el wizard arranca Y devuelve error
	// (el engine de este harness no tiene ese flow registrado). Afirmando primero
	// sobre startedFlow, el rojo dice cuál es el bug en vez de un "flow is not
	// registered" que manda a buscar al lugar equivocado.
	if s.startedFlow != "" {
		t.Fatalf("arrancó %q con el 429 ya encolado; el wizard no tiene que correr", s.startedFlow)
	}
	if err != nil {
		t.Fatalf("StartSubcategorySetup: %v", err)
	}
}

// La contracara: un error que NO es de cupo sí tiene que caer al wizard. Sin
// este par, el test de arriba se puede satisfacer rompiendo el fallback.
func TestStartSubcategorySetup_OtherError_FallsBackToWizard(t *testing.T) {
	s := &testServices{
		engine:      conversation.NewEngine(&fakeStateStore{}, func(string) string { return "algo" }),
		classifyErr: errFake,
		groqHandled: false,
	}

	_ = StartSubcategorySetup(context.Background(), s, &messenger.FakeChat{}, 1, "creá gastos de regalos")
	if s.startedFlow != flow.SubcategorySetupFlowName {
		t.Errorf("startedFlow = %q, want %q: un error común no puede dejar al usuario sin camino",
			s.startedFlow, flow.SubcategorySetupFlowName)
	}
}
