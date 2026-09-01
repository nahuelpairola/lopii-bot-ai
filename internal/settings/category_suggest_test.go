package settings

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

func TestMergeSuggestionText_WithSamples(t *testing.T) {
	got := mergeSuggestionText("Comida", "Delivery", "", []string{"PedidosYa", "Rappi"})
	want := "Comida / Delivery — gastos en: PedidosYa, Rappi"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMergeSuggestionText_DescriptionAndSamples(t *testing.T) {
	got := mergeSuggestionText("Comida", "Delivery", "Pedidos a domicilio", []string{"Rappi"})
	want := "Comida / Delivery — Pedidos a domicilio — gastos en: Rappi"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSuggestMergeTarget_ExcludesSourceFromTaxonomy(t *testing.T) {
	all := []subcategory.Subcategory{
		ownedSub(7, "Comida", "Delivery", "🍕"),
		ownedSub(3, "Alimentos", "Delivery", "🥑"),
		ownedSub(1, "Sistema", "Saldo inicial", "⚙️"),
	}
	s := &testServices{
		all:      all,
		match:    &orchestrator.CategoryMatch{Category: "Alimentos", Subcategory: "Delivery"},
		byCatSub: map[string]*subcategory.Subcategory{"Alimentos|Delivery": &all[1]},
	}

	data := conversation.Data{conversation.KeySourceCategory: "Comida", conversation.KeySourceSubcategory: "Delivery"}
	SuggestMergeTarget(context.Background(), s, 1, 7, data)

	for _, e := range s.gotTaxonomy {
		if e.Category == "Comida" && e.Subcategory == "Delivery" {
			t.Error("la taxonomía incluyó la fila de origen")
		}
		if subcategory.IsReserved(e.Category) {
			t.Errorf("la taxonomía incluyó una reservada: %s", e.Category)
		}
	}
	if len(s.gotTaxonomy) != 1 {
		t.Errorf("len(taxonomy) = %d, want 1", len(s.gotTaxonomy))
	}
}

func TestSuggestMergeTarget_ValidMatchReturnsRow(t *testing.T) {
	all := []subcategory.Subcategory{
		ownedSub(7, "Comida", "Delivery", "🍕"),
		ownedSub(3, "Alimentos", "Delivery", "🥑"),
	}
	s := &testServices{
		all:      all,
		match:    &orchestrator.CategoryMatch{Category: "Alimentos", Subcategory: "Delivery"},
		byCatSub: map[string]*subcategory.Subcategory{"Alimentos|Delivery": &all[1]},
	}

	got := SuggestMergeTarget(context.Background(), s, 1, 7, conversation.Data{})
	if got == nil || got.ID != 3 {
		t.Fatalf("got = %+v, want la fila 3", got)
	}
}

func TestSuggestMergeTarget_MatchResolvingToSourceIsDiscarded(t *testing.T) {
	all := []subcategory.Subcategory{ownedSub(7, "Comida", "Delivery", "🍕")}
	s := &testServices{
		all:      all,
		match:    &orchestrator.CategoryMatch{Category: "Comida", Subcategory: "Delivery"},
		byCatSub: map[string]*subcategory.Subcategory{"Comida|Delivery": &all[0]},
	}

	if got := SuggestMergeTarget(context.Background(), s, 1, 7, conversation.Data{}); got != nil {
		t.Errorf("got = %+v, want nil (se sugirió a sí misma)", got)
	}
}

func TestSuggestMergeTarget_ProposalInsteadOfMatchIsNoSuggestion(t *testing.T) {
	s := &testServices{
		all:      []subcategory.Subcategory{ownedSub(7, "Comida", "Delivery", "")},
		proposal: &orchestrator.CategoryProposal{Category: "Nueva", Subcategory: "Cosa"},
	}

	if got := SuggestMergeTarget(context.Background(), s, 1, 7, conversation.Data{}); got != nil {
		t.Errorf("got = %+v, want nil (Proposal no es destino de fusión)", got)
	}
}

func TestSuggestMergeTarget_OrchestratorErrorIsNoSuggestion(t *testing.T) {
	s := &testServices{
		all:         []subcategory.Subcategory{ownedSub(7, "Comida", "Delivery", "")},
		classifyErr: errFake,
	}

	if got := SuggestMergeTarget(context.Background(), s, 1, 7, conversation.Data{}); got != nil {
		t.Errorf("got = %+v, want nil", got)
	}
}

func TestSuggestMergeTarget_HallucinatedMatchIsDiscarded(t *testing.T) {
	s := &testServices{
		all:   []subcategory.Subcategory{ownedSub(7, "Comida", "Delivery", "")},
		match: &orchestrator.CategoryMatch{Category: "Inventada", Subcategory: "Nada"},
	}

	if got := SuggestMergeTarget(context.Background(), s, 1, 7, conversation.Data{}); got != nil {
		t.Errorf("got = %+v, want nil", got)
	}
}
