package miniapp

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/movement"
)

type stubIcons struct{}

func (stubIcons) IconForCategory(_ uint64, category string) string {
	if category == "Alimentación" {
		return "🍔"
	}
	return "📂"
}

func TestHandleCategories_RendersIcons(t *testing.T) {
	gin.SetMode(gin.TestMode)
	movements := stubMovements{rows: map[string][]movement.CategorySum{
		"category": {{Label: "Alimentación", Total: decimal.NewFromInt(5000)}},
	}}
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/categories"))

	if !bodyContains(w.Body.String(), "🍔") {
		t.Fatal("cada categoría debe llevar su icono")
	}
}

func TestHandleCategories_ExcludesReservedCategories(t *testing.T) {
	gin.SetMode(gin.TestMode)
	movements := stubMovements{rows: map[string][]movement.CategorySum{
		"category": {
			{Label: "Alimentación", Total: decimal.NewFromInt(5000)},
			{Label: "Sistema", Total: decimal.NewFromInt(999999)},
		},
	}}
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/categories"))

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if bodyContains(w.Body.String(), "Sistema") {
		t.Fatal("reserved category 'Sistema' must be excluded from the Categorías view")
	}
	if !bodyContains(w.Body.String(), "Alimentaci") {
		t.Fatal("expected the real category to render")
	}
}

func TestHandleCategoryDrill_CategoryWithSlash(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// "Deudas / préstamos" es una categoría real de la taxonomía. Con el drill
	// en el path, la barra rompía el routing y daba 404.
	movements := stubMovements{rows: map[string][]movement.CategorySum{
		"subcategory": {{Label: "Cuota préstamo", Total: decimal.NewFromInt(8000)}},
	}}
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/categories?category=Deudas+%2F+pr%C3%A9stamos"))

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !bodyContains(w.Body.String(), "Cuota") {
		t.Fatal("el drill debe mostrar las subcategorías de la categoría pedida")
	}
}

func TestHandleCategories_ShowsShareOfTotal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	movements := stubMovements{rows: map[string][]movement.CategorySum{
		"category": {
			{Label: "Alimentación", Total: decimal.NewFromInt(750)},
			{Label: "Transporte", Total: decimal.NewFromInt(250)},
		},
	}}
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/categories"))

	body := w.Body.String()
	if !bodyContains(body, "75%") {
		t.Fatal("cada fila debe mostrar su participación en el total")
	}
	if !bodyContains(body, "$1.000") {
		t.Fatal("la vista debe mostrar el total del período")
	}
}

func bodyContains(body, substr string) bool {
	return len(body) > 0 && (func() bool {
		for i := 0; i+len(substr) <= len(body); i++ {
			if body[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}
