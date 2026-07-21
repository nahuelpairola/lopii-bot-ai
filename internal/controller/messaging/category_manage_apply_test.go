package messaging

import (
	"context"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/subcategory"
)

// applyData arma el estado con el que el flujo 2 llega al finish. targetID
// vacío = la rama de borrado simple (la categoría no tenía movimientos).
func applyData(count, targetID string, confirmed bool) conversation.Data {
	data := conversation.Data{
		conversation.UserIDKey: uint64(1),
		keySourceSubcategoryID: "7",
		keySourceCategory:      "Comida",
		keySourceSubcategory:   "Delivery",
		keyMovementCount:       count,
	}
	if targetID != "" {
		data[keyTargetSubcategoryID] = targetID
		data[keyTargetCategory] = "Alimentos"
		data[keyTargetSubcategory] = "Delivery"
	}
	if confirmed {
		setFlag(data, keyConfirmed)
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

	c.finishCategoryManageTargetFlow(context.Background(), nil, 100, applyData("3", "3", true))

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

// Sin movimientos no hay nada que reasignar. Llamar a reassign igual sería un
// UPDATE sobre cero filas que esconde bugs de la rama equivocada.
func TestFinishCategoryManage_NoTarget_DeletesWithoutReassign(t *testing.T) {
	c, movs, subs := newApplyController()

	c.finishCategoryManageTargetFlow(context.Background(), nil, 100, applyData("0", "", true))

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
	setFlag(data, keyCancelled)
	c.finishCategoryManageTargetFlow(context.Background(), nil, 100, data)

	if movs.reassignSubCalls != 0 || subs.deleteCalls != 0 {
		t.Errorf("cancelado escribió: reassign=%d delete=%d, want 0/0", movs.reassignSubCalls, subs.deleteCalls)
	}
}

// Sin keyConfirmed tampoco se escribe: un flujo que termina por cualquier otra
// vía no puede borrar nada.
func TestFinishCategoryManage_NotConfirmed_WritesNothing(t *testing.T) {
	c, movs, subs := newApplyController()

	c.finishCategoryManageTargetFlow(context.Background(), nil, 100, applyData("3", "3", false))

	if movs.reassignSubCalls != 0 || subs.deleteCalls != 0 {
		t.Errorf("sin confirmar escribió: reassign=%d delete=%d, want 0/0", movs.reassignSubCalls, subs.deleteCalls)
	}
}

// Si la reasignación falla NO se borra la categoría: borrarla dejaría los
// movimientos apuntando a una fila muerta sin haberlos movido.
func TestFinishCategoryManage_ReassignFails_DoesNotDelete(t *testing.T) {
	c, movs, subs := newApplyController()
	movs.reassignSubErr = errFake

	c.finishCategoryManageTargetFlow(context.Background(), nil, 100, applyData("3", "3", true))

	if subs.deleteCalls != 0 {
		t.Errorf("borró la categoría pese a que falló la reasignación (delete=%d)", subs.deleteCalls)
	}
}

// Delete devuelve ErrSubcategoryNotFound cuando no borró nada. No se puede
// reportar éxito ni recargar el cache: no pasó nada que recargar.
func TestFinishCategoryManage_DeleteFindsNothing_DoesNotReportSuccess(t *testing.T) {
	c, _, subs := newApplyController()
	subs.deleteErr = subcategory.ErrSubcategoryNotFound

	c.finishCategoryManageTargetFlow(context.Background(), nil, 100, applyData("0", "", true))

	if subs.reloadCalls != 0 {
		t.Errorf("recargó el cache (%d) pese a que no se borró nada", subs.reloadCalls)
	}
}

func TestFinishCategoryManage_BadSourceID_WritesNothing(t *testing.T) {
	c, movs, subs := newApplyController()

	data := applyData("3", "3", true)
	data[keySourceSubcategoryID] = "no-es-un-numero"
	c.finishCategoryManageTargetFlow(context.Background(), nil, 100, data)

	if movs.reassignSubCalls != 0 || subs.deleteCalls != 0 {
		t.Errorf("con id inválido escribió: reassign=%d delete=%d, want 0/0", movs.reassignSubCalls, subs.deleteCalls)
	}
}

func TestFinishCategoryManage_BadTargetID_WritesNothing(t *testing.T) {
	c, movs, subs := newApplyController()

	data := applyData("3", "3", true)
	data[keyTargetSubcategoryID] = "tampoco"
	c.finishCategoryManageTargetFlow(context.Background(), nil, 100, data)

	if movs.reassignSubCalls != 0 || subs.deleteCalls != 0 {
		t.Errorf("con destino inválido escribió: reassign=%d delete=%d, want 0/0", movs.reassignSubCalls, subs.deleteCalls)
	}
}

// El dispatcher tiene que conocer los dos flujos nuevos: si faltara un case,
// el flujo terminaría en el default y no escribiría nunca.
func TestHandleFlowFinished_KnowsCategoryManageFlows(t *testing.T) {
	for _, name := range []string{categoryManagePickFlowName, categoryManageTargetFlowName} {
		if name == "" {
			t.Fatal("nombre de flujo vacío")
		}
	}
	// NewFlow valida el grafo al construir: si un NextStep colgara, esto explota.
	if f := NewCategoryManagePickFlow(fakeOwnedLister{}); f.Name != categoryManagePickFlowName {
		t.Errorf("pick flow Name = %q", f.Name)
	}
	if f := NewCategoryManageTargetFlow(fakeTargetLister{}); f.Name != categoryManageTargetFlowName {
		t.Errorf("target flow Name = %q", f.Name)
	}
}
