package miniapp

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/movement"
)

type stubMovementsMatrix struct {
	stubMovementsWithAccounts
	byMonth map[string][]movement.CategorySum // keyed by "YYYY-MM"
}

func (s stubMovementsMatrix) SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error) {
	if groupBy == "subcategory" {
		key := q.From.Format("2006-01")
		return s.byMonth[key], nil
	}
	return nil, nil
}

func TestHandleMatrix_RendersTableFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	currentMonth := time.Now().Format("2006-01")
	movements := stubMovementsMatrix{byMonth: map[string][]movement.CategorySum{
		currentMonth: {{Label: "Supermercado", Total: decimal.NewFromInt(1500)}},
	}}
	c := NewController(movements, stubAccountsWithData{}, stubUsers{}, testBotToken)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/matrix"))

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !bodyContains(w.Body.String(), "<table>") {
		t.Fatal("expected an accessible <table> fallback alongside the heatmap")
	}
}
