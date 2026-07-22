package messaging

import (
	"context"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/subcategory"
)

func TestMergeSuggestionText_NamesOnly(t *testing.T) {
	got := mergeSuggestionText("Comida", "Delivery", "", nil)
	want := "Comida / Delivery"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMergeSuggestionText_WithDescription(t *testing.T) {
	got := mergeSuggestionText("Comida", "Delivery", "Pedidos a domicilio", nil)
	want := "Comida / Delivery — Pedidos a domicilio"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMergeSuggestionText_WithMerchants(t *testing.T) {
	got := mergeSuggestionText("Comida", "Delivery", "", []string{"PedidosYa", "Rappi"})
	want := "Comida / Delivery — gastos en: PedidosYa, Rappi"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMergeSuggestionText_DescriptionAndMerchants(t *testing.T) {
	got := mergeSuggestionText("Comida", "Delivery", "Pedidos a domicilio", []string{"Rappi"})
	want := "Comida / Delivery — Pedidos a domicilio — gastos en: Rappi"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// fakeCategoryOrchestrator embebe movementOrchestrator: los métodos que no se
// usan quedan nil y explotan si alguien los llama por error.
type fakeCategoryOrchestrator struct {
	movementOrchestrator
	match       *orchestrator.CategoryMatch
	proposal    *orchestrator.CategoryProposal
	err         error
	calls       int
	gotText     string
	gotTaxonomy []orchestrator.TaxonomyEntry
}

func (f *fakeCategoryOrchestrator) ClassifyCategoryCreate(_ context.Context, text string, taxonomy []orchestrator.TaxonomyEntry) (orchestrator.CategoryCreateResult, error) {
	f.calls++
	f.gotText, f.gotTaxonomy = text, taxonomy
	if f.err != nil {
		return orchestrator.CategoryCreateResult{}, f.err
	}
	return orchestrator.CategoryCreateResult{Match: f.match, Proposal: f.proposal}, nil
}

// La taxonomía que se le manda al LLM NO puede incluir la fila de origen, o se
// sugeriría a sí misma.
func TestSuggestMergeTarget_ExcludesSourceFromTaxonomy(t *testing.T) {
	all := []subcategory.Subcategory{
		ownedSub(7, "Comida", "Delivery", "🍕"),
		ownedSub(3, "Alimentos", "Delivery", "🥑"),
		ownedSub(1, "Sistema", "Saldo inicial", "⚙️"),
	}
	orch := &fakeCategoryOrchestrator{
		match: &orchestrator.CategoryMatch{Category: "Alimentos", Subcategory: "Delivery"},
	}
	subs := &fakeSubcategoryRepoFull{
		all:              all,
		byCategoryAndSub: map[string]*subcategory.Subcategory{"Alimentos|Delivery": &all[1]},
	}
	c := &controller{orchestrator: orch, subcategories: subs, movements: &fakeMovementRepoFull{}}

	data := conversation.Data{keySourceCategory: "Comida", keySourceSubcategory: "Delivery"}
	c.suggestMergeTarget(context.Background(), 1, 7, data)

	for _, e := range orch.gotTaxonomy {
		if e.Category == "Comida" && e.Subcategory == "Delivery" {
			t.Error("la taxonomía incluyó la fila de origen")
		}
		if subcategory.IsReserved(e.Category) {
			t.Errorf("la taxonomía incluyó una reservada: %s", e.Category)
		}
	}
	if len(orch.gotTaxonomy) != 1 {
		t.Errorf("len(taxonomy) = %d, want 1", len(orch.gotTaxonomy))
	}
}

func TestSuggestMergeTarget_ValidMatchReturnsRow(t *testing.T) {
	all := []subcategory.Subcategory{
		ownedSub(7, "Comida", "Delivery", "🍕"),
		ownedSub(3, "Alimentos", "Delivery", "🥑"),
	}
	orch := &fakeCategoryOrchestrator{
		match: &orchestrator.CategoryMatch{Category: "Alimentos", Subcategory: "Delivery"},
	}
	subs := &fakeSubcategoryRepoFull{
		all:              all,
		byCategoryAndSub: map[string]*subcategory.Subcategory{"Alimentos|Delivery": &all[1]},
	}
	c := &controller{orchestrator: orch, subcategories: subs, movements: &fakeMovementRepoFull{}}

	got := c.suggestMergeTarget(context.Background(), 1, 7, conversation.Data{})
	if got == nil || got.ID != 3 {
		t.Fatalf("got = %+v, want la fila 3", got)
	}
}

func TestSuggestMergeTarget_MatchResolvingToSourceIsDiscarded(t *testing.T) {
	all := []subcategory.Subcategory{ownedSub(7, "Comida", "Delivery", "🍕")}
	orch := &fakeCategoryOrchestrator{
		match: &orchestrator.CategoryMatch{Category: "Comida", Subcategory: "Delivery"},
	}
	subs := &fakeSubcategoryRepoFull{
		all:              all,
		byCategoryAndSub: map[string]*subcategory.Subcategory{"Comida|Delivery": &all[0]},
	}
	c := &controller{orchestrator: orch, subcategories: subs, movements: &fakeMovementRepoFull{}}

	if got := c.suggestMergeTarget(context.Background(), 1, 7, conversation.Data{}); got != nil {
		t.Errorf("got = %+v, want nil (se sugirió a sí misma)", got)
	}
}

func TestSuggestMergeTarget_ProposalInsteadOfMatchIsNoSuggestion(t *testing.T) {
	orch := &fakeCategoryOrchestrator{
		proposal: &orchestrator.CategoryProposal{Category: "Nueva", Subcategory: "Cosa"},
	}
	subs := &fakeSubcategoryRepoFull{all: []subcategory.Subcategory{ownedSub(7, "Comida", "Delivery", "")}}
	c := &controller{orchestrator: orch, subcategories: subs, movements: &fakeMovementRepoFull{}}

	if got := c.suggestMergeTarget(context.Background(), 1, 7, conversation.Data{}); got != nil {
		t.Errorf("got = %+v, want nil (Proposal no es destino de fusión)", got)
	}
}

func TestSuggestMergeTarget_OrchestratorErrorIsNoSuggestion(t *testing.T) {
	orch := &fakeCategoryOrchestrator{err: errFake}
	subs := &fakeSubcategoryRepoFull{all: []subcategory.Subcategory{ownedSub(7, "Comida", "Delivery", "")}}
	c := &controller{orchestrator: orch, subcategories: subs, movements: &fakeMovementRepoFull{}}

	if got := c.suggestMergeTarget(context.Background(), 1, 7, conversation.Data{}); got != nil {
		t.Errorf("got = %+v, want nil", got)
	}
}

// Un match alucinado (que no existe en la taxonomía del usuario) no se ofrece.
func TestSuggestMergeTarget_HallucinatedMatchIsDiscarded(t *testing.T) {
	orch := &fakeCategoryOrchestrator{
		match: &orchestrator.CategoryMatch{Category: "Inventada", Subcategory: "Nada"},
	}
	subs := &fakeSubcategoryRepoFull{
		all: []subcategory.Subcategory{ownedSub(7, "Comida", "Delivery", "")},
		// byCategoryAndSub sin entrada para "Inventada|Nada": FindByCategoryAndSubcategory
		// devuelve ErrSubcategoryNotFound, tal como haría la Cache real ante una alucinación.
	}
	c := &controller{orchestrator: orch, subcategories: subs, movements: &fakeMovementRepoFull{}}

	if got := c.suggestMergeTarget(context.Background(), 1, 7, conversation.Data{}); got != nil {
		t.Errorf("got = %+v, want nil", got)
	}
}
