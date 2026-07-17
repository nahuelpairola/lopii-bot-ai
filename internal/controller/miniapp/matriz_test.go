package miniapp

import (
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/movement"
)

type stubMovementsMatriz struct {
	stubMovementsWithAccounts
	byMonth map[string][]movement.CategorySum // keyed by "YYYY-MM"
}

func (s stubMovementsMatriz) SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error) {
	if groupBy == "subcategory" {
		key := q.From.Format("2006-01")
		return s.byMonth[key], nil
	}
	return nil, nil
}

func TestHandleMatriz_RendersTableFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	currentMonth := time.Now().Format("2006-01")
	movements := stubMovementsMatriz{byMonth: map[string][]movement.CategorySum{
		currentMonth: {{Label: "Supermercado", Total: decimal.NewFromInt(1500)}},
	}}
	c := NewController(movements, stubAccountsWithData{}, stubUsers{}, testBotToken)
	router := gin.New()
	c.RegisterRoutes(router)

	initData := buildInitData(t, "999", timeNow(), testBotToken)
	req := httptest.NewRequest("GET", "/app/matriz?tgWebAppData="+url.QueryEscape(initData), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !bodyContains(w.Body.String(), "<table>") {
		t.Fatal("expected an accessible <table> fallback alongside the heatmap")
	}
}
