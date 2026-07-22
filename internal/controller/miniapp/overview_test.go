package miniapp

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/controller/miniapp/templates"
	"lopiibot.com/internal/currency"
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

func TestHandleOverview_RendersOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	movements := stubMovements{rows: map[string][]movement.CategorySum{
		"":      {{Label: "", Total: decimal.NewFromInt(1000)}},
		"month": {{Label: "2026-07", Total: decimal.NewFromInt(1000)}},
	}}
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken)

	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/overview"))

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleOverview_MonthWindowUsesDailyGrouping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Con ventana de un mes el trend agrupa por día y va con UNA serie:
	// 31 días × 2 series serían 62 barras en un teléfono.
	movements := stubMovements{rows: map[string][]movement.CategorySum{
		"":    {{Label: "", Total: decimal.NewFromInt(1000)}},
		"day": {{Label: "2026-07-03", Total: decimal.NewFromInt(400)}},
	}}
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/overview?p=month"))

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !bodyContains(w.Body.String(), "2026-07-03") {
		t.Fatal("con ventana de mes el trend debe venir agrupado por día")
	}
	if bodyContains(w.Body.String(), `"label":"Ingresos"`) {
		t.Fatal("la ventana de mes va con una sola serie (gastos)")
	}
}

func TestHandleOverview_FormatsMoneyAndShowsPeriod(t *testing.T) {
	gin.SetMode(gin.TestMode)
	movements := stubMovements{rows: map[string][]movement.CategorySum{
		"": {{Label: "", Total: decimal.NewFromInt(1234567)}},
	}}
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/overview?p=6m"))

	body := w.Body.String()
	if !bodyContains(body, "$1.234.567") {
		t.Fatal("los montos deben salir en formato AR")
	}
	// El label se computa contra el mes corriente para que el test no
	// caduque al cambiar de mes.
	current := templates.CurrentMonth(nowInART())
	want := templates.NewPeriod(templates.RouteOverview, templates.Preset6M, current, current, currency.ARS, templates.AllPresets).Label
	if !bodyContains(body, want) {
		t.Fatalf("el período %q debe estar escrito en pantalla", want)
	}
	if !bodyContains(body, `id="app-state"`) {
		t.Fatal("el partial debe traer el estado para que lo lea la tab bar")
	}
}

func TestHandleOverview_FullPageNav_ServesShellUnauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := NewController(stubMovements{}, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken)
	router := gin.New()
	c.RegisterRoutes(router)

	// No HX-Request, no initData — a plain browser navigation. Must NOT 401;
	// it serves the shell, which then self-loads the authed content.
	req := httptest.NewRequest("GET", "/app/overview", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200 shell, got %d", w.Code)
	}
	if !bodyContains(w.Body.String(), `hx-get="/app/overview"`) {
		t.Fatal("shell must self-load its content via htmx")
	}
}

func TestHandleOverview_HTMXWithoutInitData_401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := NewController(stubMovements{}, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken)
	router := gin.New()
	c.RegisterRoutes(router)

	// htmx request but no initData header — the attacker/out-of-Telegram case.
	req := httptest.NewRequest("GET", "/app/overview", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401 for htmx request without initData, got %d", w.Code)
	}
}
