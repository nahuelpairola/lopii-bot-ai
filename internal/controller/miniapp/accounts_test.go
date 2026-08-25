package miniapp

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/controller/miniapp/templates"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

type stubMovementsWithAccounts struct {
	stubMovements
	balances map[uint64]decimal.Decimal
	deltas   map[uint64][]movement.MonthlyDelta
	// movements es lo que devuelve ListForAccount: las filas de la hoja.
	movements []movement.Movement
}

func (s stubMovementsWithAccounts) ListForAccount(accountID uint64, from, to time.Time, limit int) ([]movement.Movement, error) {
	return s.movements, nil
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
	c := NewController(movements, stubAccountsTwoCurrencies{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
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
	c := NewController(movements, stubAccountsWithData{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
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

func TestHandleAccountLeaf_ReconcilesBalance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	desc := "Compra de dólares"
	accountID := uint64(1)
	movements := stubMovementsWithAccounts{
		balances: map[uint64]decimal.Decimal{1: decimal.NewFromInt(50000)},
		deltas: map[uint64][]movement.MonthlyDelta{
			// Lo de julio es el saldo con el que abre agosto.
			1: {{Month: "2026-07", Delta: decimal.NewFromInt(80000)}, {Month: "2026-08", Delta: decimal.NewFromInt(-30000)}},
		},
		// A propósito la lista NO suma lo mismo que el delta del mes (-30.000):
		// simula el corte del tope. Si el cierre se calculara sumando las filas
		// visibles daría $70.000, y este test lo caza.
		movements: []movement.Movement{{
			AccountID: &accountID, Date: time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
			Type: movement.Transfer, Amount: decimal.NewFromInt(-10000), Currency: currency.ARS,
			Description: &desc,
		}},
	}
	c := NewController(movements, stubAccountsWithData{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/accounts?p=month&m=2026-08&c=ARS&account=1"))
	body := w.Body.String()

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, body)
	}
	if !bodyContains(body, "$80.000") {
		t.Errorf("el saldo de apertura es lo acumulado ANTES de la ventana:\n%s", body)
	}
	if !bodyContains(body, "$50.000") {
		t.Errorf("el cierre es apertura + el acumulado del período (80.000-30.000), nunca la suma de las filas visibles:\n%s", body)
	}
	if bodyContains(body, "$70.000") {
		t.Error("el cierre se calculó sumando las filas listadas: con el tope de filas eso da mal")
	}
	if !bodyContains(body, "Compra de dólares") {
		t.Errorf("una compra de USD tiene que aparecer, es lo que apply escondía:\n%s", body)
	}
}

func TestHandleAccountLeaf_RejectsAnotherUsersAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	movements := stubMovementsWithAccounts{
		balances: map[uint64]decimal.Decimal{},
		deltas:   map[uint64][]movement.MonthlyDelta{},
	}
	// stubAccountsWithData sólo devuelve la cuenta 1.
	c := NewController(movements, stubAccountsWithData{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/accounts?account=99"))

	if w.Code != 404 {
		t.Fatalf("una cuenta que no es del usuario tiene que dar 404, dio %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleAccountLeaf_LabelsUnclassified(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accountID := uint64(1)
	movements := stubMovementsWithAccounts{
		balances: map[uint64]decimal.Decimal{1: decimal.Zero},
		deltas:   map[uint64][]movement.MonthlyDelta{1: {}},
		movements: []movement.Movement{{
			AccountID: &accountID, Date: time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC),
			Type: movement.Expense, Amount: decimal.NewFromInt(-1500), Currency: currency.ARS,
			Subcategory: &subcategory.Subcategory{Category: constants.PendingReview, Subcategory: "algo"},
		}},
	}
	c := NewController(movements, stubAccountsWithData{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/accounts?p=month&m=2026-08&c=ARS&account=1"))
	body := w.Body.String()

	if !bodyContains(body, "Sin clasificar") {
		t.Errorf("un movimiento sin clasificar mueve el saldo: hay que mostrarlo como tal:\n%s", body)
	}
	if bodyContains(body, constants.PendingReview) {
		t.Errorf("PENDING_REVIEW es jerga interna, no puede llegar a la pantalla:\n%s", body)
	}
}

func TestHandleAccountLeaf_SurvivesNilSubcategory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accountID := uint64(1)
	movements := stubMovementsWithAccounts{
		balances: map[uint64]decimal.Decimal{1: decimal.Zero},
		deltas:   map[uint64][]movement.MonthlyDelta{1: {}},
		movements: []movement.Movement{{
			AccountID: &accountID, Date: time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC),
			Type: movement.Expense, Amount: decimal.NewFromInt(-1500), Currency: currency.ARS,
			// Sin Description y sin Subcategory: los dos son punteros.
		}},
	}
	c := NewController(movements, stubAccountsWithData{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/accounts?p=month&m=2026-08&c=ARS&account=1"))

	if w.Code != 200 {
		t.Fatalf("una fila sin descripción ni subcategoría no puede tumbar la vista, dio %d", w.Code)
	}
	if !bodyContains(w.Body.String(), templates.RowFallbackTitle) {
		t.Error("esa fila tiene que rendir con el título genérico")
	}
}

func TestTaxonomyNote(t *testing.T) {
	cases := []struct {
		name        string
		category    string
		subcategory string
		title       string
		want        string
	}{
		{"el par completo", "Alimentación", "Supermercado", "Coto", "Alimentación › Supermercado"},
		{"sin clasificar gana sobre todo lo demás", constants.PendingReview, "algo", "Coto", msgUnclassified},
		{"no repite la subcategoría cuando ya es el título", "Alimentación", "Supermercado", "Supermercado", "Alimentación"},
		{"sin subcategoría queda la categoría", "Alimentación", "", "Coto", "Alimentación"},
		{"sin taxonomía no inventa nada", "", "", "Coto", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := taxonomyNote(c.category, c.subcategory, c.title); got != c.want {
				t.Errorf("taxonomyNote = %q, want %q", got, c.want)
			}
		})
	}
}

// leafKeepsDrill busca un link de la cabecera que cambie el rango Y conserve la
// hoja. Ancla en p=6m porque ese preset sale sólo de WithPreset: los links de
// vuelta y los de las filas llevan el período actual. Desescapa el & primero,
// que templ escapa dentro de los atributos.
//
// El "pt=6m" del medio no es decorativo: el drill viaja después de TODOS los
// params del período (url.Values ordena las claves), así que si el link se
// armara con un solo ámbito esta aserción no matchearía. La hoja es una vista
// single-period, así que su chip escribe "p" y "pt" queda en su default.
func leafKeepsDrill(body, drill string) bool {
	return bodyContains(strings.ReplaceAll(body, "&amp;", "&"), "p=6m&pt=6m"+drill)
}

func TestHandleAccountLeaf_PeriodChipsKeepTheAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accountID := uint64(1)
	movements := stubMovementsWithAccounts{
		balances: map[uint64]decimal.Decimal{1: decimal.NewFromInt(50000)},
		deltas:   map[uint64][]movement.MonthlyDelta{1: {{Month: "2026-07", Delta: decimal.NewFromInt(50000)}}},
		movements: []movement.Movement{{
			AccountID: &accountID, Date: time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
			Type: movement.Expense, Amount: decimal.NewFromInt(-1000), Currency: currency.ARS,
		}},
	}
	c := NewController(movements, stubAccountsWithData{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/accounts?account=1&p=month&m=2026-07"))

	if !leafKeepsDrill(w.Body.String(), "&account=1") {
		t.Error("cambiar el rango dentro de la hoja pierde account=1 y vuelve al índice")
	}
}

func TestHandleAccountLeaf_HidesTheCurrencyChips(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accountID := uint64(1)
	movements := stubMovementsWithAccounts{
		balances: map[uint64]decimal.Decimal{1: decimal.NewFromInt(50000)},
		deltas:   map[uint64][]movement.MonthlyDelta{1: {{Month: "2026-07", Delta: decimal.NewFromInt(50000)}}},
		movements: []movement.Movement{{
			AccountID: &accountID, Date: time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
			Type: movement.Expense, Amount: decimal.NewFromInt(-1000), Currency: currency.ARS,
		}},
	}
	c := NewController(movements, stubAccountsWithData{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/accounts?account=1&p=month&m=2026-07"))

	// Una cuenta tiene UNA moneda: el chip USD dejaría la hoja de una cuenta en
	// ARS con el período en USD, que es peor que perder el drill.
	if bodyContains(strings.ReplaceAll(w.Body.String(), "&amp;", "&"), "c=USD") {
		t.Error("la hoja de una cuenta no debe ofrecer cambiar de moneda")
	}
}

// El índice de Cuentas comparte el slot "p" con Resumen y Categorías: cambiar
// el rango en Evolución (que vive en "pt") no puede arrastrarlo.
func TestHandleAccounts_SharesThePeriodOfOverview(t *testing.T) {
	gin.SetMode(gin.TestMode)
	movements := stubMovementsWithAccounts{
		balances: map[uint64]decimal.Decimal{1: decimal.NewFromInt(50000)},
		deltas: map[uint64][]movement.MonthlyDelta{
			1: {{Month: "2026-06", Delta: decimal.NewFromInt(30000)}, {Month: "2026-07", Delta: decimal.NewFromInt(20000)}},
		},
	}
	c := NewController(movements, stubAccountsWithData{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	// "Mes" en el slot de un período, "año" en el de tendencia: manda el
	// primero, y el gráfico se va porque un mes no dibuja una tendencia.
	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/accounts?p=month&pt=year"))
	body := w.Body.String()

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, body)
	}
	if !appStateHas(body, "p", "month") {
		t.Error("Cuentas debe leer el slot de un período, no el de tendencia")
	}
	if !bodyContains(body, "$50.000") {
		t.Error("las tarjetas son saldo de hoy: no dependen del período")
	}
	if bodyContains(body, "cuentas-trend") {
		t.Error("con ventana de un mes el gráfico es un punto suelto: no se dibuja")
	}

	// Con una ventana que sí es una tendencia, el gráfico vuelve.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/accounts?p=6m"))
	if !bodyContains(w.Body.String(), "cuentas-trend") {
		t.Error("con 6M el gráfico de tendencia tiene que estar")
	}
}

// Dos cuentas en la MISMA moneda: es lo unico que hace visible un total
// distinto de un saldo suelto. stubAccountsWithData devuelve una sola, y
// stubAccountsTwoCurrencies devuelve dos que el handler filtra a una.
type stubAccountsTwoARS struct{}

func (stubAccountsTwoARS) FindByUserID(userID uint64) ([]account.Account, error) {
	efectivo := account.Account{Name: "Efectivo", Currency: currency.ARS}
	efectivo.ID = 1
	banco := account.Account{Name: "Banco", Currency: currency.ARS}
	banco.ID = 2
	return []account.Account{efectivo, banco}, nil
}

// El total es la suma de las tarjetas que estan abajo, no una consulta nueva:
// se acumula en el loop que ya suma cada cuenta. Por eso cierra por
// construccion — y por eso el test lo verifica contra la suma de los saldos
// que la misma pantalla muestra.
func TestHandleAccounts_TotalsTheBalancesItShows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	movements := stubMovementsWithAccounts{
		balances: map[uint64]decimal.Decimal{
			1: decimal.NewFromInt(50000),
			2: decimal.NewFromInt(25500),
		},
	}
	c := NewController(movements, stubAccountsTwoARS{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/accounts"))

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !bodyContains(body, "$75.500") {
		t.Errorf("falta el total $75.500 (50.000 + 25.500), que es lo que hace que la pantalla cierre:\n%s", body)
	}
}
