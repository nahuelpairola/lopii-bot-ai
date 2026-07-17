package miniapp

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/user"
)

type stubMovements struct {
	rows map[string][]movement.CategorySum // keyed by groupBy, for simplicity in this test
}

func (s stubMovements) SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error) {
	return s.rows[groupBy], nil
}

type stubAccounts struct{}

func (stubAccounts) FindByUserID(userID uint64) ([]account.Account, error) { return nil, nil }

type stubUsers struct{}

func (stubUsers) FindByTelegramID(telegramID string) (*user.User, error) {
	return &user.User{ID: 1, TelegramID: telegramID}, nil
}

func TestHandleResumen_RendersOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	movements := stubMovements{rows: map[string][]movement.CategorySum{
		"":      {{Label: "", Total: decimal.NewFromInt(1000)}},
		"month": {{Label: "2026-07", Total: decimal.NewFromInt(1000)}},
	}}
	c := NewController(movements, stubAccounts{}, stubUsers{}, testBotToken)

	router := gin.New()
	c.RegisterRoutes(router)

	initData := buildInitData(t, "999", timeNow(), testBotToken)
	req := httptest.NewRequest("GET", "/app/resumen?tgWebAppData="+url.QueryEscape(initData), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
