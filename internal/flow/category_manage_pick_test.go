package flow

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
	fl := NewCategoryManagePickFlow(fakeOwnedLister{})
	if fl.Name != CategoryManagePickFlowName {
		t.Errorf("Name = %q, want %q", fl.Name, CategoryManagePickFlowName)
	}
	if fl.InitialStep != StepPickSource {
		t.Errorf("InitialStep = %q, want %q", fl.InitialStep, StepPickSource)
	}
}

func TestCategoryManagePickFlow_ListsOneOptionPerOwnedRow(t *testing.T) {
	engine, store := newCategoryManagePickEngine(twoOwned())
	prompt, err := engine.StartWithData(1, CategoryManagePickFlowName, conversation.Data{})
	if err != nil {
		t.Fatalf("StartWithData: %v", err)
	}
	if store.stepName != StepPickSource {
		t.Fatalf("stepName = %q, want %q", store.stepName, StepPickSource)
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
	if prompt.Buttons[2].Data != OptionCancel {
		t.Errorf("último botón = %q, want %q", prompt.Buttons[2].Data, OptionCancel)
	}
}

func TestCategoryManagePickFlow_ChoosingSourceFinishesWithIDAndNames(t *testing.T) {
	engine, _ := newCategoryManagePickEngine(twoOwned())
	engine.StartWithData(1, CategoryManagePickFlowName, conversation.Data{})

	result, found, err := engine.Handle(1, conversation.Input{CallbackData: "7"})
	if err != nil || !found {
		t.Fatalf("Handle: found=%v err=%v", found, err)
	}
	if !result.Finished {
		t.Fatal("elegir el origen debería terminar el flujo 1")
	}
	if got := conversation.StringOrEmpty(result.Data[conversation.KeySourceSubcategoryID]); got != "7" {
		t.Errorf("source id = %q, want %q", got, "7")
	}
	if got := conversation.StringOrEmpty(result.Data[conversation.KeySourceCategory]); got != "Comida" {
		t.Errorf("source category = %q, want %q", got, "Comida")
	}
	if got := conversation.StringOrEmpty(result.Data[conversation.KeySourceSubcategory]); got != "Delivery" {
		t.Errorf("source subcategory = %q, want %q", got, "Delivery")
	}
	if conversation.Flag(result.Data, conversation.KeyCancelled) {
		t.Error("elegir no debería marcar cancelado")
	}
}

func TestCategoryManagePickFlow_CancelMarksCancelledAndSetsNoSource(t *testing.T) {
	engine, _ := newCategoryManagePickEngine(twoOwned())
	engine.StartWithData(1, CategoryManagePickFlowName, conversation.Data{})

	result, _, err := engine.Handle(1, conversation.Input{CallbackData: OptionCancel})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !result.Finished {
		t.Fatal("cancelar debería terminar el flujo")
	}
	if !conversation.Flag(result.Data, conversation.KeyCancelled) {
		t.Error("cancelar debería marcar conversation.KeyCancelled")
	}
	if conversation.StringOrEmpty(result.Data[conversation.KeySourceSubcategoryID]) != "" {
		t.Error("cancelar no debería dejar un origen seteado")
	}
}

// raceLister simula la ventana de la corrección de Arreglo 2: el cache
// devuelve la fila en las dos primeras consultas (armado de botones +
// validación de la opción elegida, adentro de ChoiceStep) pero ya no en la
// tercera (la que OnChoice hace para sacar los nombres) — como si un
// Reload() concurrente la hubiese sacado justo en el medio.
type raceLister struct {
	calls  *int
	before []subcategory.Subcategory
	after  []subcategory.Subcategory
	missAt int
}

func (r raceLister) FindOwnedByUser(uint64) ([]subcategory.Subcategory, error) {
	*r.calls++
	if *r.calls >= r.missAt {
		return r.after, nil
	}
	return r.before, nil
}

func TestCategoryManagePickFlow_RowDisappearsBetweenQueries_NoPartialSource(t *testing.T) {
	calls := 0
	lister := raceLister{
		calls:  &calls,
		before: twoOwned(),                                                          // fila 7 presente (armado de botones + match)
		after:  []subcategory.Subcategory{ownedSub(9, "Regalos", "Cumpleaños", "")}, // fila 7 ya no está (lookup de nombres en OnChoice)
		missAt: 3,
	}
	store := &fakeStateStore{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewCategoryManagePickFlow(lister))

	if _, err := engine.StartWithData(1, CategoryManagePickFlowName, conversation.Data{}); err != nil {
		t.Fatalf("StartWithData: %v", err)
	}

	result, found, err := engine.Handle(1, conversation.Input{CallbackData: "7"})
	if err != nil || !found {
		t.Fatalf("Handle: found=%v err=%v", found, err)
	}
	if !result.Finished {
		t.Fatal("elegir un id que matcheó debería terminar el flujo 1")
	}
	if got := conversation.StringOrEmpty(result.Data[conversation.KeySourceSubcategoryID]); got != "" {
		t.Errorf("la fila desapareció en la segunda consulta de OnChoice: no debería quedar conversation.KeySourceSubcategoryID (got %q)", got)
	}
	if got := conversation.StringOrEmpty(result.Data[conversation.KeySourceCategory]); got != "" {
		t.Errorf("no debería quedar conversation.KeySourceCategory sin su ID (got %q)", got)
	}
	if got := conversation.StringOrEmpty(result.Data[conversation.KeySourceSubcategory]); got != "" {
		t.Errorf("no debería quedar conversation.KeySourceSubcategory sin su ID (got %q)", got)
	}
}

func TestCategoryManagePickFlow_UnknownCallbackRetries(t *testing.T) {
	engine, _ := newCategoryManagePickEngine(twoOwned())
	engine.StartWithData(1, CategoryManagePickFlowName, conversation.Data{})

	result, _, err := engine.Handle(1, conversation.Input{CallbackData: "999"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.Finished {
		t.Error("un id desconocido no debería terminar el flujo")
	}
}
