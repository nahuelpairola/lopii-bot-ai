package messaging

import (
	"context"
	"strings"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/subcategory"
)

func applyData(count, targetID string, confirmed bool) conversation.Data {
	data := conversation.Data{
		conversation.UserIDKey:              uint64(1),
		conversation.KeySourceSubcategoryID: "7",
		conversation.KeySourceCategory:      "Comida",
		conversation.KeySourceSubcategory:   "Delivery",
		conversation.KeyMovementCount:       count,
	}
	if targetID != "" {
		data[conversation.KeyTargetSubcategoryID] = targetID
		data[conversation.KeyTargetCategory] = "Alimentos"
		data[conversation.KeyTargetSubcategory] = "Delivery"
	}
	if confirmed {
		conversation.SetFlag(data, conversation.KeyConfirmed)
	}
	return data
}

func newApplyController() (*controller, *fakeMovementRepoFull, *fakeSubcategoryRepoFull) {
	movs := &fakeMovementRepoFull{}
	subs := &fakeSubcategoryRepoFull{}
	return &controller{movements: movs, subcategories: subs, metrics: &fakeMetricRepo{}}, movs, subs
}

func TestFinishCategoryManage_WithTarget_ReassignsThenDeletes(t *testing.T) {
	c, movs, subs := newApplyController()

	c.finishCategoryManageTargetFlow(context.Background(), &messenger.FakeChat{}, applyData("3", "3", true))

	if movs.reassignSubCalls != 1 {
		t.Fatalf("reassign llamado %d veces, want 1", movs.reassignSubCalls)
	}
	if movs.reassignedFrom != 7 || movs.reassignedTo != 3 {
		t.Errorf("reassign(%d → %d), want (7 → 3)", movs.reassignedFrom, movs.reassignedTo)
	}
	if movs.reassignedUser != 1 {
		t.Errorf("reassign userID = %d, want 1", movs.reassignedUser)
	}
	if subs.deleteCalls != 1 {
		t.Fatalf("delete llamado %d veces, want 1", subs.deleteCalls)
	}
	if subs.deletedID != 7 || subs.deletedUserID != 1 {
		t.Errorf("delete(user=%d, id=%d), want (1, 7)", subs.deletedUserID, subs.deletedID)
	}
	if subs.reloadCalls != 1 {
		t.Errorf("reload llamado %d veces, want 1 (sin esto la categoría borrada sigue apareciendo)", subs.reloadCalls)
	}
}

func TestFinishCategoryManage_NoTarget_DeletesWithoutReassign(t *testing.T) {
	c, movs, subs := newApplyController()

	c.finishCategoryManageTargetFlow(context.Background(), &messenger.FakeChat{}, applyData("0", "", true))

	if movs.reassignSubCalls != 0 {
		t.Errorf("reassign llamado %d veces sin destino, want 0", movs.reassignSubCalls)
	}
	if subs.deleteCalls != 1 {
		t.Errorf("delete llamado %d veces, want 1", subs.deleteCalls)
	}
}

func TestFinishCategoryManage_Cancelled_WritesNothing(t *testing.T) {
	c, movs, subs := newApplyController()

	data := applyData("3", "3", false)
	conversation.SetFlag(data, conversation.KeyCancelled)
	c.finishCategoryManageTargetFlow(context.Background(), &messenger.FakeChat{}, data)

	if movs.reassignSubCalls != 0 || subs.deleteCalls != 0 {
		t.Errorf("cancelado escribió: reassign=%d delete=%d, want 0/0", movs.reassignSubCalls, subs.deleteCalls)
	}
}

func TestFinishCategoryManage_NotConfirmed_WritesNothing(t *testing.T) {
	c, movs, subs := newApplyController()

	c.finishCategoryManageTargetFlow(context.Background(), &messenger.FakeChat{}, applyData("3", "3", false))

	if movs.reassignSubCalls != 0 || subs.deleteCalls != 0 {
		t.Errorf("sin confirmar escribió: reassign=%d delete=%d, want 0/0", movs.reassignSubCalls, subs.deleteCalls)
	}
}

func TestFinishCategoryManage_ReassignFails_DoesNotDelete(t *testing.T) {
	c, movs, subs := newApplyController()
	movs.reassignSubErr = errFake

	c.finishCategoryManageTargetFlow(context.Background(), &messenger.FakeChat{}, applyData("3", "3", true))

	if subs.deleteCalls != 0 {
		t.Errorf("borró la categoría pese a que falló la reasignación (delete=%d)", subs.deleteCalls)
	}
}

func TestFinishCategoryManage_DeleteFindsNothing_DoesNotReportSuccess(t *testing.T) {
	c, _, subs := newApplyController()
	subs.deleteErr = subcategory.ErrSubcategoryNotFound

	c.finishCategoryManageTargetFlow(context.Background(), &messenger.FakeChat{}, applyData("0", "", true))

	if subs.reloadCalls != 0 {
		t.Errorf("recargó el cache (%d) pese a que no se borró nada", subs.reloadCalls)
	}
}

func TestFinishCategoryManage_BadSourceID_WritesNothing(t *testing.T) {
	c, movs, subs := newApplyController()

	data := applyData("3", "3", true)
	data[conversation.KeySourceSubcategoryID] = "no-es-un-numero"
	c.finishCategoryManageTargetFlow(context.Background(), &messenger.FakeChat{}, data)

	if movs.reassignSubCalls != 0 || subs.deleteCalls != 0 {
		t.Errorf("con id inválido escribió: reassign=%d delete=%d, want 0/0", movs.reassignSubCalls, subs.deleteCalls)
	}
}

func TestFinishCategoryManage_BadTargetID_WritesNothing(t *testing.T) {
	c, movs, subs := newApplyController()

	data := applyData("3", "3", true)
	data[conversation.KeyTargetSubcategoryID] = "tampoco"
	c.finishCategoryManageTargetFlow(context.Background(), &messenger.FakeChat{}, data)

	if movs.reassignSubCalls != 0 || subs.deleteCalls != 0 {
		t.Errorf("con destino inválido escribió: reassign=%d delete=%d, want 0/0", movs.reassignSubCalls, subs.deleteCalls)
	}
}

func TestHandleFlowFinished_KnowsCategoryManageFlows(t *testing.T) {
	for _, name := range []string{flow.CategoryManagePickFlowName, flow.CategoryManageTargetFlowName} {
		if name == "" {
			t.Fatal("nombre de flujo vacío")
		}
	}
	if f := flow.NewCategoryManagePickFlow(fakeOwnedLister{}); f.Name != flow.CategoryManagePickFlowName {
		t.Errorf("pick flow Name = %q", f.Name)
	}
	if f := flow.NewCategoryManageTargetFlow(fakeTargetLister{}); f.Name != flow.CategoryManageTargetFlowName {
		t.Errorf("target flow Name = %q", f.Name)
	}
}

func newCategoryManageE2E(count int64, match *orchestrator.CategoryMatch) (*controller, *fakeMovementRepoFull, *fakeSubcategoryRepoFull) {
	catalog := []subcategory.Subcategory{
		ownedSub(7, "Comida", "Delivery", "🍕"),
		ownedSub(3, "Alimentos", "Delivery", "🥑"),
		ownedSub(4, "Alimentos", "Supermercado", "🛒"),
	}
	subs := &fakeSubcategoryRepoFull{
		all:        catalog,
		owned:      catalog,
		categories: []string{"Alimentos", "Comida"},
		byCategoryAndSub: map[string]*subcategory.Subcategory{
			"Alimentos|Delivery": &catalog[1],
		},
	}
	movs := &fakeMovementRepoFull{countBySubcategory: count}

	store := &fakeStateStore{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(flow.NewCategoryManagePickFlow(subs))
	engine.Register(flow.NewCategoryManageTargetFlow(subs))

	c := &controller{
		engine:        engine,
		movements:     movs,
		subcategories: subs,
		orchestrator:  &fakeCategoryOrchestrator{match: match},
		metrics:       &fakeMetricRepo{},
	}
	return c, movs, subs
}

func TestCategoryManageE2E_MergePath(t *testing.T) {
	match := &orchestrator.CategoryMatch{Category: "Alimentos", Subcategory: "Delivery"}
	c, movs, subs := newCategoryManageE2E(3, match)
	const userID = uint64(1)

	if _, err := c.engine.Start(userID, flow.CategoryManagePickFlowName); err != nil {
		t.Fatalf("Start flujo 1: %v", err)
	}

	res1, _, err := c.engine.Handle(userID, conversation.Input{CallbackData: "7"})
	if err != nil || !res1.Finished {
		t.Fatalf("elegir origen: finished=%v err=%v", res1.Finished, err)
	}

	c.finishCategoryManagePickFlow(context.Background(), &messenger.FakeChat{}, res1.Data)

	res2, found, err := c.engine.Handle(userID, conversation.Input{CallbackData: flow.OptionAcceptSuggestion})
	if err != nil || !found {
		t.Fatalf("aceptar sugerencia: found=%v err=%v (¿arrancó el flujo 2?)", found, err)
	}
	if res2.Finished {
		t.Fatal("aceptar la sugerencia no debería terminar el flujo: falta confirmar")
	}

	res3, _, err := c.engine.Handle(userID, conversation.Input{CallbackData: flow.OptionConfirm})
	if err != nil || !res3.Finished {
		t.Fatalf("confirmar: finished=%v err=%v", res3.Finished, err)
	}

	c.finishCategoryManageTargetFlow(context.Background(), &messenger.FakeChat{}, res3.Data)

	if movs.reassignSubCalls != 1 || movs.reassignedFrom != 7 || movs.reassignedTo != 3 {
		t.Errorf("reassign: calls=%d from=%d to=%d, want 1/7/3",
			movs.reassignSubCalls, movs.reassignedFrom, movs.reassignedTo)
	}
	if subs.deleteCalls != 1 || subs.deletedID != 7 {
		t.Errorf("delete: calls=%d id=%d, want 1/7", subs.deleteCalls, subs.deletedID)
	}
}

func TestCategoryManageE2E_EmptyDeletePath(t *testing.T) {
	c, movs, subs := newCategoryManageE2E(0, nil)
	const userID = uint64(1)

	c.engine.Start(userID, flow.CategoryManagePickFlowName)
	res1, _, err := c.engine.Handle(userID, conversation.Input{CallbackData: "7"})
	if err != nil || !res1.Finished {
		t.Fatalf("elegir origen: %v", err)
	}
	c.finishCategoryManagePickFlow(context.Background(), &messenger.FakeChat{}, res1.Data)

	res2, found, err := c.engine.Handle(userID, conversation.Input{CallbackData: flow.OptionConfirm})
	if err != nil || !found {
		t.Fatalf("confirmar borrado: found=%v err=%v", found, err)
	}
	if !res2.Finished {
		t.Fatal("confirmar debería terminar el flujo")
	}

	c.finishCategoryManageTargetFlow(context.Background(), &messenger.FakeChat{}, res2.Data)

	if movs.reassignSubCalls != 0 {
		t.Errorf("reasignó %d veces sin movimientos, want 0", movs.reassignSubCalls)
	}
	if subs.deleteCalls != 1 || subs.deletedID != 7 {
		t.Errorf("delete: calls=%d id=%d, want 1/7", subs.deleteCalls, subs.deletedID)
	}
}

func TestCategoryManageE2E_ManualPath_ConfirmShowsTargetName(t *testing.T) {
	match := &orchestrator.CategoryMatch{Category: "Alimentos", Subcategory: "Delivery"}
	c, movs, _ := newCategoryManageE2E(3, match)
	const userID = uint64(1)

	c.engine.Start(userID, flow.CategoryManagePickFlowName)
	res1, _, _ := c.engine.Handle(userID, conversation.Input{CallbackData: "7"})
	c.finishCategoryManagePickFlow(context.Background(), &messenger.FakeChat{}, res1.Data)

	c.engine.Handle(userID, conversation.Input{CallbackData: flow.OptionChooseOther})
	c.engine.Handle(userID, conversation.Input{CallbackData: "Alimentos"})
	res, _, err := c.engine.Handle(userID, conversation.Input{CallbackData: "4"})
	if err != nil {
		t.Fatalf("elegir subcategoría destino: %v", err)
	}

	if !strings.Contains(res.Prompt.Text, "Supermercado") {
		t.Errorf("el confirm no nombra el destino elegido.\ngot: %q", res.Prompt.Text)
	}

	res3, _, _ := c.engine.Handle(userID, conversation.Input{CallbackData: flow.OptionConfirm})
	c.finishCategoryManageTargetFlow(context.Background(), &messenger.FakeChat{}, res3.Data)

	if movs.reassignedTo != 4 {
		t.Errorf("reasignó a %d, want 4 (el elegido a mano, no el sugerido)", movs.reassignedTo)
	}
}
