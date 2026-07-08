package messaging

import (
	"context"
	"testing"

	"gorm.io/gorm"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/subcategory"
)


func TestOnboardingRowsFromDrafts_DefaultsAndSkips(t *testing.T) {
	drafts := []orchestrator.OnboardingAccountDraft{
		{Name: "Banco", Currency: "ARS", Balance: "20000"},
		{Name: "", Currency: "", Balance: "5000"},        // → name Efectivo, currency ARS
		{Name: "Bróker", Currency: "USD", Balance: "100"},
		{Name: "Acciones", Currency: "USD", Balance: ""}, // no balance → skipped
	}
	rows := onboardingRowsFromDrafts(drafts)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3 (unparseable-balance row skipped)", len(rows))
	}
	if rows[1].Name != "Efectivo" || rows[1].Currency != "ARS" {
		t.Errorf("row 1 = %+v, want Efectivo/ARS defaults", rows[1])
	}
}

func TestMarkDefaults_FirstPerCurrency(t *testing.T) {
	rows := markDefaults([]onboardingRow{
		{Name: "Banco", Currency: "ARS"},
		{Name: "Efectivo", Currency: "ARS"},
		{Name: "Bróker", Currency: "USD"},
	})
	if rows[0].IsDefault != "true" || rows[1].IsDefault != "false" || rows[2].IsDefault != "true" {
		t.Errorf("defaults = %q/%q/%q, want true/false/true", rows[0].IsDefault, rows[1].IsDefault, rows[2].IsDefault)
	}
}

func TestEncodeDecodeOnboardingRows_RoundTrip(t *testing.T) {
	rows := []onboardingRow{{Name: "Banco", Currency: "ARS", Balance: "20000", IsDefault: "true"}}
	data := conversation.Data{"accounts": encodeOnboardingRows(rows)}
	got := decodeOnboardingRows(data)
	if len(got) != 1 || got[0].Name != "Banco" || got[0].IsDefault != "true" {
		t.Errorf("round trip = %+v, want the original row", got)
	}
}

func TestFinishOnboardingConfirmFlow_ConfirmInserts(t *testing.T) {
	subRepo := &fakeSubcategoryRepoFull{
		byCategoryAndSub: map[string]*subcategory.Subcategory{
			"Sistema|Saldo inicial": {Model: gorm.Model{ID: 7}, Category: "Sistema", Subcategory: "Saldo inicial"},
		},
	}
	movRepo := &fakeMovementRepoFull{}
	c := &controller{subcategories: subRepo, movements: movRepo}

	rows := markDefaults([]onboardingRow{
		{Name: "Banco", Currency: "ARS", Balance: "20000"},
		{Name: "Bróker", Currency: "USD", Balance: "100"},
	})
	data := conversation.Data{conversation.UserIDKey: uint64(1), "accounts": encodeOnboardingRows(rows)}

	c.finishOnboardingConfirmFlow(context.Background(), nil, 0, data)

	if len(movRepo.openings) != 2 {
		t.Fatalf("inserted %d account-openings, want 2", len(movRepo.openings))
	}
	if !movRepo.openings[0].Account.IsDefault || !movRepo.openings[1].Account.IsDefault {
		t.Errorf("default flags wrong: first-ARS should be default, USD is a different currency so also default")
	}
}
