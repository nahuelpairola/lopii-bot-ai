package messaging

import (
	"encoding/json"
	"testing"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/subcategory"
)

// El modelo devuelve de a ratos el par entero ("Vivienda | Luz") en el campo
// categoría. ClassifyCreate lo corrige desde siempre; el agent loop desarma los
// argumentos de record_movements por su cuenta y NO lo hacía, así que desde que
// CREATE va por el loop la corrección estaba muerta: cada movimiento así abría
// un gap y el bot le preguntaba al usuario la categoría que ya había escrito.
//
// Reproducido en producción local el 2026-08-04 (traza 23d52b13): las DOS filas
// del mensaje abrieron gap.
func TestAgentRecord_NormalizesTheCategoryPair(t *testing.T) {
	arsAccount := acct(46, currency.ARS, true)
	accRepo := &fakeAccountRepoFull{
		byCurrency: map[currency.Currency]*account.Account{currency.ARS: &arsAccount},
		byUserID:   []account.Account{arsAccount},
	}
	subRepo := &fakeSubcategoryRepoFull{byCategoryAndSub: map[string]*subcategory.Subcategory{
		"Vivienda|Luz": newSubForTest(99, "Vivienda", "Luz"),
	}}
	c := &controller{accounts: accRepo, subcategories: subRepo, movements: &fakeMovementRepoFull{
		balances: map[uint64]string{46: "500000"},
	}}

	e := &agentExecutor{
		c:        c,
		userID:   1,
		userText: "pagué 12000 de luz",
		taxonomy: []orchestrator.TaxonomyEntry{{Category: "Vivienda", Subcategory: "Luz"}},
	}

	args := json.RawMessage(`{"movements":[{
		"type":"expense","amount":"12000","currency":"ARS",
		"category":"Vivienda | Luz","subcategory":"Luz","date":"2026-08-04"
	}]}`)

	if _, err := e.record(args); err != nil && err != orchestrator.ErrAgentTurnDone {
		t.Fatalf("record: %v", err)
	}

	// Sin normalizar, el par no matchea la taxonomía, buildCreateSeed abre gap
	// de categoría y record parkea en vez de escribir.
	if len(e.parked) != 0 {
		t.Fatalf("parkeó %d acciones: el par no se normalizó y abrió gap", len(e.parked))
	}
	if !e.wrote || len(e.inserted) != 1 {
		t.Fatalf("no insertó: wrote=%v inserted=%d", e.wrote, len(e.inserted))
	}
	if got := e.inserted[0].SubcategoryID; got != 99 {
		t.Errorf("subcategory_id = %d, want 99 (Vivienda | Luz)", got)
	}
}
