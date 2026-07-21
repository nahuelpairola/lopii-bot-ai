package messaging

import (
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/subcategory"
)

type fakeOwnedLister struct {
	owned []subcategory.Subcategory
	err   error
}

func (f fakeOwnedLister) FindOwnedByUser(uint64) ([]subcategory.Subcategory, error) {
	return f.owned, f.err
}

func ownedSub(id uint, category, sub, icon string) subcategory.Subcategory {
	s := subcategory.Subcategory{Category: category, Subcategory: sub, Icon: icon}
	s.ID = id
	return s
}

func newCategoryManagePickEngine(owned []subcategory.Subcategory) (*conversation.Engine, *fakeStateStore) {
	store := &fakeStateStore{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewCategoryManagePickFlow(fakeOwnedLister{owned: owned}))
	return engine, store
}

func twoOwned() []subcategory.Subcategory {
	return []subcategory.Subcategory{
		ownedSub(7, "Comida", "Delivery", "🍕"),
		ownedSub(9, "Regalos", "Cumpleaños", ""),
	}
}

// El flujo construye sin panic: NewFlow valida el grafo al registrar, así que
// un NextStep colgado explota acá y no en producción.
func TestCategoryManagePickFlow_BuildsWithoutPanic(t *testing.T) {
	flow := NewCategoryManagePickFlow(fakeOwnedLister{})
	if flow.Name != categoryManagePickFlowName {
		t.Errorf("Name = %q, want %q", flow.Name, categoryManagePickFlowName)
	}
	if flow.InitialStep != stepPickSource {
		t.Errorf("InitialStep = %q, want %q", flow.InitialStep, stepPickSource)
	}
}

func TestCategoryManagePickFlow_ListsOneOptionPerOwnedRow(t *testing.T) {
	engine, store := newCategoryManagePickEngine(twoOwned())
	prompt, err := engine.StartWithData(1, categoryManagePickFlowName, conversation.Data{})
	if err != nil {
		t.Fatalf("StartWithData: %v", err)
	}
	if store.stepName != stepPickSource {
		t.Fatalf("stepName = %q, want %q", store.stepName, stepPickSource)
	}
	if len(prompt.Buttons) != 3 {
		t.Fatalf("len(Buttons) = %d, want 3 (2 propias + cancelar)", len(prompt.Buttons))
	}
	if prompt.Buttons[0].Label != "🍕 Comida › Delivery" {
		t.Errorf("Buttons[0].Label = %q, want %q", prompt.Buttons[0].Label, "🍕 Comida › Delivery")
	}
	if prompt.Buttons[0].Data != "7" {
		t.Errorf("Buttons[0].Data = %q, want %q", prompt.Buttons[0].Data, "7")
	}
	// sin ícono propio cae al genérico, nunca a un label sin prefijo
	if prompt.Buttons[1].Label != "📂 Regalos › Cumpleaños" {
		t.Errorf("Buttons[1].Label = %q, want %q", prompt.Buttons[1].Label, "📂 Regalos › Cumpleaños")
	}
	if prompt.Buttons[2].Data != optionCancel {
		t.Errorf("último botón = %q, want %q", prompt.Buttons[2].Data, optionCancel)
	}
}

func TestCategoryManagePickFlow_ChoosingSourceFinishesWithIDAndNames(t *testing.T) {
	engine, _ := newCategoryManagePickEngine(twoOwned())
	engine.StartWithData(1, categoryManagePickFlowName, conversation.Data{})

	result, found, err := engine.Handle(1, conversation.Input{CallbackData: "7"})
	if err != nil || !found {
		t.Fatalf("Handle: found=%v err=%v", found, err)
	}
	if !result.Finished {
		t.Fatal("elegir el origen debería terminar el flujo 1")
	}
	if got := stringOrEmpty(result.Data[keySourceSubcategoryID]); got != "7" {
		t.Errorf("source id = %q, want %q", got, "7")
	}
	if got := stringOrEmpty(result.Data[keySourceCategory]); got != "Comida" {
		t.Errorf("source category = %q, want %q", got, "Comida")
	}
	if got := stringOrEmpty(result.Data[keySourceSubcategory]); got != "Delivery" {
		t.Errorf("source subcategory = %q, want %q", got, "Delivery")
	}
	if flag(result.Data, keyCancelled) {
		t.Error("elegir no debería marcar cancelado")
	}
}

func TestCategoryManagePickFlow_CancelMarksCancelledAndSetsNoSource(t *testing.T) {
	engine, _ := newCategoryManagePickEngine(twoOwned())
	engine.StartWithData(1, categoryManagePickFlowName, conversation.Data{})

	result, _, err := engine.Handle(1, conversation.Input{CallbackData: optionCancel})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !result.Finished {
		t.Fatal("cancelar debería terminar el flujo")
	}
	if !flag(result.Data, keyCancelled) {
		t.Error("cancelar debería marcar keyCancelled")
	}
	if stringOrEmpty(result.Data[keySourceSubcategoryID]) != "" {
		t.Error("cancelar no debería dejar un origen seteado")
	}
}

func TestCategoryManagePickFlow_UnknownCallbackRetries(t *testing.T) {
	engine, _ := newCategoryManagePickEngine(twoOwned())
	engine.StartWithData(1, categoryManagePickFlowName, conversation.Data{})

	result, _, err := engine.Handle(1, conversation.Input{CallbackData: "999"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.Finished {
		t.Error("un id desconocido no debería terminar el flujo")
	}
}
