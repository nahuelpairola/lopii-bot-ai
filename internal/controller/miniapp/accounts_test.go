package miniapp

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

type stubMovementsWithAccounts struct {
	stubMovements
	balances map[uint64]decimal.Decimal
	deltas   map[uint64][]movement.MonthlyDelta
}

func (s stubMovementsWithAccounts) SumAmountForAccount(accountID uint64) (decimal.Decimal, error) {
	return s.balances[accountID], nil
}

func (s stubMovementsWithAccounts) MonthlyDeltasForAccount(accountID uint64) ([]movement.MonthlyDelta, error) {
	return s.deltas[accountID], nil
}

type stubAccountsWithData struct{}

func (stubAccountsWithData) FindByUserID(userID uint64) ([]account.Account, error) {
	acct := account.Account{Name: "Efectivo", Currency: currency.ARS}
	acct.ID = 1
	return []account.Account{acct}, nil
}

type stubAccountsTwoCurrencies struct{}

func (stubAccountsTwoCurrencies) FindByUserID(userID uint64) ([]account.Account, error) {
	ars := account.Account{Name: "Efectivo", Currency: currency.ARS}
	ars.ID = 1
	usd := account.Account{Name: "Dólares", Currency: currency.USD}
	usd.ID = 2
	return []account.Account{ars, usd}, nil
}

func TestHandleAccounts_NeverMixesCurrencies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	movements := stubMovementsWithAccounts{
		balances: map[uint64]decimal.Decimal{1: decimal.NewFromInt(50000), 2: decimal.NewFromInt(500)},
		deltas: map[uint64][]movement.MonthlyDelta{
			1: {{Month: "2026-07", Delta: decimal.NewFromInt(50000)}},
			2: {{Month: "2026-07", Delta: decimal.NewFromInt(500)}},
		},
	}
	c := NewController(movements, stubAccountsTwoCurrencies{}, stubIcons{}, stubUsers{}, testBotToken)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/accounts?c=ARS"))
	body := w.Body.String()

	if !bodyContains(body, "Efectivo") {
		t.Fatal("la cuenta en ARS debe estar")
	}
	if bodyContains(body, "Dólares") {
		t.Fatal("una cuenta en USD no puede compartir eje con una en ARS")
	}
}

func TestHandleAccounts_RendersBalances(t *testing.T) {
	gin.SetMode(gin.TestMode)
	movements := stubMovementsWithAccounts{
		balances: map[uint64]decimal.Decimal{1: decimal.NewFromInt(50000)},
		deltas: map[uint64][]movement.MonthlyDelta{
			1: {{Month: "2026-06", Delta: decimal.NewFromInt(30000)}, {Month: "2026-07", Delta: decimal.NewFromInt(20000)}},
		},
	}
	c := NewController(movements, stubAccountsWithData{}, stubIcons{}, stubUsers{}, testBotToken)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/accounts"))

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !bodyContains(w.Body.String(), "$50.000") {
		t.Fatal("expected the account balance to render, AR-formatted")
	}
}
