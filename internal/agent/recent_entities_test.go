package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

func TestRenderRecentEntities(t *testing.T) {
	desc := "Café"
	m := movement.Movement{
		Model:       gorm.Model{ID: 250, CreatedAt: time.Now().Add(-6 * time.Minute)},
		Description: &desc,
		Amount:      decimal.RequireFromString("-12700"),
		Currency:    currency.ARS,
		Account:     &account.Account{Name: "Galicia"},
	}

	block := renderRecentEntities([]movement.Movement{m})

	for _, want := range []string{"#250", "Café", "12.700", "Galicia", "6 min"} {
		if !strings.Contains(block, want) {
			t.Errorf("el bloque no contiene %q:\n%s", want, block)
		}
	}
	if strings.Contains(block, "-12") {
		t.Errorf("el signo contable se escapó al prompt:\n%s", block)
	}
}

func TestRenderRecentEntities_EmptyWhenNothingRecent(t *testing.T) {
	if got := renderRecentEntities(nil); got != "" {
		t.Errorf("sin movimientos el bloque tiene que ser vacío, got %q", got)
	}
}

func TestRenderRecentEntities_FallsBackToSubcategory(t *testing.T) {
	m := movement.Movement{
		Model:       gorm.Model{ID: 7, CreatedAt: time.Now()},
		Amount:      decimal.RequireFromString("-500"),
		Currency:    currency.ARS,
		Subcategory: &subcategory.Subcategory{Category: "Alimentación", Subcategory: "Supermercado"},
	}
	if got := renderRecentEntities([]movement.Movement{m}); !strings.Contains(got, "Supermercado") {
		t.Errorf("want el fallback a la subcategoría, got %q", got)
	}
}
