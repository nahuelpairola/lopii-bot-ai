package messaging

import (
	"testing"

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
		// Sin acentos: el usuario escribe rápido y el teclado del teléfono no ayuda.
		{"sin acentos", "seguro vehiculo", "Transporte", "Seguro vehículo", true},
		// Sólo lo inequívoco: adivinar mal acá es un dato corrupto, y el gap-fill
		// ya sabe preguntar.
		{"no existe", "proyecto hogar", "", "", false},
		{"vacío", "   ", "", "", false},
		// "hogar" solo NO alcanza: hay dos. Un match parcial elegiría una al azar.
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

// El lote se trata como lote SÓLO con las dos condiciones puestas. Sin el cambio
// estructurado hay que volver al picker: la alternativa es pedirle al modelo que
// reproduzca N filas enteras, que es donde corrompe datos en silencio.
func TestIsBatchCorrection(t *testing.T) {
	twoCandidates := []candidateGroup{{OldIDs: []string{"1"}}, {OldIDs: []string{"2"}}}
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
		// Un solo candidato no necesita el camino de lote: ya se confirma directo.
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
