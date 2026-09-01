package agent

import (
	"testing"

	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/orchestrator"
)

func testTaxonomy() []orchestrator.TaxonomyEntry {
	return []orchestrator.TaxonomyEntry{
		{Category: "Alimentación", Subcategory: "Supermercado"},
		{Category: "Vivienda", Subcategory: "Mantenimiento hogar"},
		{Category: "Vivienda", Subcategory: "Seguro hogar"},
		{Category: "Transporte", Subcategory: "Seguro vehículo"},
	}
}

func TestResolveTaxonomyPair(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantCat string
		wantSub string
		wantOK  bool
	}{
		{"sólo la subcategoría", "mantenimiento hogar", "Vivienda", "Mantenimiento hogar", true},
		{"el par entero", "Vivienda | Mantenimiento hogar", "Vivienda", "Mantenimiento hogar", true},
		{"con barra", "vivienda/mantenimiento hogar", "Vivienda", "Mantenimiento hogar", true},
		{"sin acentos", "seguro vehiculo", "Transporte", "Seguro vehículo", true},
		{"no existe", "proyecto hogar", "", "", false},
		{"vacío", "   ", "", "", false},
		{"ambiguo", "hogar", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cat, sub, ok := resolveTaxonomyPair(tc.input, testTaxonomy())
			if ok != tc.wantOK || cat != tc.wantCat || sub != tc.wantSub {
				t.Errorf("resolveTaxonomyPair(%q) = %q/%q/%v, want %q/%q/%v",
					tc.input, cat, sub, ok, tc.wantCat, tc.wantSub, tc.wantOK)
			}
		})
	}
}

func TestIsBatchCorrection(t *testing.T) {
	twoCandidates := []flow.CandidateGroup{{OldIDs: []string{"1"}}, {OldIDs: []string{"2"}}}
	change := []correctionChange{{fieldCategory, opSet, "Vivienda"}}

	cases := []struct {
		name    string
		payload agentPayload
		want    bool
	}{
		{"lote", agentPayload{Scope: scopeAll, Changes: change, Candidates: twoCandidates}, true},
		{"sin cambio estructurado", agentPayload{Scope: scopeAll, Candidates: twoCandidates}, false},
		{"scope one", agentPayload{Scope: scopeOne, Changes: change, Candidates: twoCandidates}, false},
		{"scope vacío", agentPayload{Changes: change, Candidates: twoCandidates}, false},
		{"un candidato", agentPayload{Scope: scopeAll, Changes: change, Candidates: twoCandidates[:1]}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isBatchCorrection(tc.payload); got != tc.want {
				t.Errorf("isBatchCorrection = %v, want %v", got, tc.want)
			}
		})
	}
}
