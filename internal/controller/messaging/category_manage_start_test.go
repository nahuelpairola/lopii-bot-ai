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
