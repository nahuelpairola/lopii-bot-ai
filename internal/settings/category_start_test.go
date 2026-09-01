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

func TestStartSubcategorySetup_RateLimited_SkipsWizard(t *testing.T) {
	s := &testServices{
		engine:      conversation.NewEngine(&fakeStateStore{}, func(string) string { return "algo" }),
		classifyErr: errFake,
		groqHandled: true,
	}

	err := StartSubcategorySetup(context.Background(), s, &messenger.FakeChat{}, 1, "creá gastos de regalos")

	if s.startedFlow != "" {
		t.Fatalf("arrancó %q con el 429 ya encolado; el wizard no tiene que correr", s.startedFlow)
	}
	if err != nil {
		t.Fatalf("StartSubcategorySetup: %v", err)
	}
}

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
