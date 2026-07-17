package miniapp

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/movement"
)

func TestHandleCategorias_ExcludesReservedCategories(t *testing.T) {
	gin.SetMode(gin.TestMode)
	movements := stubMovements{rows: map[string][]movement.CategorySum{
		"category": {
			{Label: "Alimentación", Total: decimal.NewFromInt(5000)},
			{Label: "Sistema", Total: decimal.NewFromInt(999999)},
		},
	}}
	c := NewController(movements, stubAccounts{}, stubUsers{}, testBotToken)
	router := gin.New()
	c.RegisterRoutes(router)

	initData := buildInitData(t, "999", timeNow(), testBotToken)
	req := httptest.NewRequest("GET", "/app/categorias?tgWebAppData="+url.QueryEscape(initData), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

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
