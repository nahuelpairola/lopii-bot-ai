package messaging

import (
	"context"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/subcategory"
)

// Un usuario sin categorías propias vería un picker con solo "Cancelar": un
// callejón. El guard tiene que cortar antes de arrancar el flujo.
func TestStartCategoryManage_NoOwnCategories_DoesNotStartFlow(t *testing.T) {
	store := &fakeStateStore{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewCategoryManagePickFlow(fakeOwnedLister{}))

	subs := &fakeSubcategoryRepoFull{owned: nil}
	c := &controller{engine: engine, subcategories: subs}

	if err := c.startCategoryManage(context.Background(), nil, 100, 1); err != nil {
		t.Fatalf("startCategoryManage: %v", err)
	}
	if store.stepName != "" {
		t.Errorf("arrancó un flujo (%q) con cero categorías propias", store.stepName)
	}
}

func TestStartCategoryManage_WithOwnCategories_StartsPickFlow(t *testing.T) {
	owned := []subcategory.Subcategory{ownedSub(7, "Comida", "Delivery", "🍕")}
	store := &fakeStateStore{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewCategoryManagePickFlow(fakeOwnedLister{owned: owned}))

	subs := &fakeSubcategoryRepoFull{owned: owned}
	c := &controller{engine: engine, subcategories: subs}

	if err := c.startCategoryManage(context.Background(), nil, 100, 1); err != nil {
		t.Fatalf("startCategoryManage: %v", err)
	}
	if store.stepName != stepPickSource {
		t.Errorf("stepName = %q, want %q", store.stepName, stepPickSource)
	}
}

// Un error del repo no puede dejar al usuario sin respuesta ni arrancar un
// flujo a medias.
func TestStartCategoryManage_RepoError_DoesNotStartFlow(t *testing.T) {
	store := &fakeStateStore{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewCategoryManagePickFlow(fakeOwnedLister{}))

	subs := &fakeSubcategoryRepoFull{ownedErr: errFake}
	c := &controller{engine: engine, subcategories: subs}

	if err := c.startCategoryManage(context.Background(), nil, 100, 1); err == nil {
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
	store := &fakeStateStore{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewCategoryManagePickFlow(fakeOwnedLister{}))

	metrics := &fakeMetricRepo{}
	c := &controller{engine: engine, subcategories: &fakeSubcategoryRepoFull{}, metrics: metrics}

	if err := c.startCategoryManage(context.Background(), nil, 100, 1); err != nil {
		t.Fatalf("startCategoryManage: %v", err)
	}
	if len(metrics.resolved) != 1 || metrics.resolved[0] != outcomeCategoryManageNoOwn {
		t.Errorf("resolved = %v, want [%s] (si queda sin resolver, el sweeper lo marca abandoned)",
			metrics.resolved, outcomeCategoryManageNoOwn)
	}
}
