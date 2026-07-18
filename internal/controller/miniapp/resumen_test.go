package miniapp

import (
	"net/http/httptest"
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

func (s stubMovements) SumAmountForAccount(accountID uint64) (decimal.Decimal, error) {
	return decimal.Zero, nil
}

func (s stubMovements) MonthlyDeltasForAccount(accountID uint64) ([]movement.MonthlyDelta, error) {
	return nil, nil
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

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/resumen"))

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleResumen_FullPageNav_ServesShellUnauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := NewController(stubMovements{}, stubAccounts{}, stubUsers{}, testBotToken)
	router := gin.New()
	c.RegisterRoutes(router)

	// No HX-Request, no initData — a plain browser navigation. Must NOT 401;
	// it serves the shell, which then self-loads the authed content.
	req := httptest.NewRequest("GET", "/app/resumen", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200 shell, got %d", w.Code)
	}
	if !bodyContains(w.Body.String(), `hx-get="/app/resumen"`) {
		t.Fatal("shell must self-load its content via htmx")
	}
}

func TestHandleResumen_HTMXWithoutInitData_401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := NewController(stubMovements{}, stubAccounts{}, stubUsers{}, testBotToken)
	router := gin.New()
	c.RegisterRoutes(router)

	// htmx request but no initData header — the attacker/out-of-Telegram case.
	req := httptest.NewRequest("GET", "/app/resumen", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401 for htmx request without initData, got %d", w.Code)
	}
}
