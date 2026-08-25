package miniapp

import (
	"context"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

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

func (s stubMovements) ListForAccount(accountID uint64, from, to time.Time, limit int) ([]movement.Movement, error) {
	return nil, nil
}

func (s stubMovements) ListForUser(q movement.MovementQuery, limit int) ([]movement.Movement, error) {
	return nil, nil
}

type stubAccounts struct{}

func (stubAccounts) FindByUserID(userID uint64) ([]account.Account, error) { return nil, nil }

type stubUsers struct{}

func (stubUsers) FindByChannel(channel, channelUserID string) (*user.User, error) {
	return &user.User{ID: 1}, nil
}

func TestHandleOverview_RendersOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	movements := stubMovements{rows: map[string][]movement.CategorySum{
		"":      {{Label: "", Total: decimal.NewFromInt(1000)}},
		"month": {{Label: "2026-07", Total: decimal.NewFromInt(1000)}},
	}}
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)

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
	// Con ventana de un mes el trend agrupa por día, y lleva las dos series
	// igual que las ventanas más largas: un mes sin la columna de ingresos no
	// deja comparar contra lo que entró.
	movements := stubMovements{rows: map[string][]movement.CategorySum{
		"":    {{Label: "", Total: decimal.NewFromInt(1000)}},
		"day": {{Label: "2026-07-03", Total: decimal.NewFromInt(400)}},
	}}
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/overview?p=month"))

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// La clave del bucket es diaria, pero al eje llega sólo el número de día:
	// el header del período ya dice de qué mes se está hablando.
	if !bodyContains(body, `"labels":["3"]`) {
		t.Fatalf("con ventana de mes el eje va por día y sin la fecha completa; body=%s", body)
	}
	if !bodyContains(body, `"label":"Ingresos"`) {
		t.Fatal("la ventana de mes también lleva la serie de ingresos")
	}
}

func TestHandleOverview_FormatsMoneyAndShowsPeriod(t *testing.T) {
	gin.SetMode(gin.TestMode)
	movements := stubMovements{rows: map[string][]movement.CategorySum{
		"": {{Label: "", Total: decimal.NewFromInt(1234567)}},
	}}
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
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
	want := templates.NewPeriod(templates.RouteOverview, templates.SinglePeriodScope,
		map[string]string{templates.SinglePeriodScope.Param: templates.Preset6M,
			templates.TrendScope.Param: templates.Preset6M},
		current, current, currency.ARS).Label
	if !bodyContains(body, want) {
		t.Fatalf("el período %q debe estar escrito en pantalla", want)
	}
	if !bodyContains(body, `id="app-state"`) {
		t.Fatal("el partial debe traer el estado para que lo lea la tab bar")
	}
}

func TestHandleOverview_FullPageNav_ServesShellUnauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := NewController(stubMovements{}, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
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
	c := NewController(stubMovements{}, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
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

// El caso que motivó todo esto: ajustar el saldo de una cuenta de CEDEARs
// porque el valor en pesos fluctuó. Eso NO es un ingreso — mañana el CEDEAR
// baja y sale. Tiene que verse, pero en su propia línea y fuera del neto.
func TestHandleOverview_ShowsBalanceVariationSeparately(t *testing.T) {
	gin.SetMode(gin.TestMode)
	movements := stubMovements{rows: map[string][]movement.CategorySum{
		"":     {{Label: "", Total: decimal.NewFromInt(1000)}},
		"day":  {{Label: "2026-08-05", Total: decimal.NewFromInt(1000)}},
		"type": {{Label: "income", Total: decimal.NewFromInt(84200)}},
	}}
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/overview"))

	body := w.Body.String()
	if !bodyContains(body, "Variaci") {
		t.Fatal("un período con ajustes debe mostrar la fila de variación de saldos")
	}
	// Con signo explícito: es un delta, no un saldo.
	if !bodyContains(body, "+$84.200") {
		t.Fatalf("la variación debe rendirse con signo; body=%s", body)
	}
}

// Un ajuste negativo (el CEDEAR bajó) tampoco es un gasto: mismo lugar, otro signo.
func TestHandleOverview_NegativeVariationIsNotAnExpense(t *testing.T) {
	gin.SetMode(gin.TestMode)
	movements := stubMovements{rows: map[string][]movement.CategorySum{
		"":     {{Label: "", Total: decimal.NewFromInt(1000)}},
		"day":  {{Label: "2026-08-05", Total: decimal.NewFromInt(1000)}},
		"type": {{Label: "expense", Total: decimal.NewFromInt(5000)}},
	}}
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/overview"))

	if body := w.Body.String(); !bodyContains(body, "-$5.000") {
		t.Fatalf("un ajuste negativo debe rendirse como variación negativa; body=%s", body)
	}
}

// Sin ajustes no hay fila: un mes normal no gasta lugar diciendo "no pasó nada".
func TestHandleOverview_NoVariationRowWhenNoAdjustments(t *testing.T) {
	gin.SetMode(gin.TestMode)
	movements := stubMovements{rows: map[string][]movement.CategorySum{
		"":    {{Label: "", Total: decimal.NewFromInt(1000)}},
		"day": {{Label: "2026-08-05", Total: decimal.NewFromInt(1000)}},
	}}
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/overview"))

	if bodyContains(w.Body.String(), "Variaci") {
		t.Fatal("sin ajustes en el período, la fila de variación no debe aparecer")
	}
}

// Un período cuyo único evento fue un ajuste NO está vacío: el saldo se movió.
// Sin esto la vista se contradecía — mostraba la variación y abajo "Sin
// movimientos en este período".
func TestHandleOverview_VariationAloneIsNotAnEmptyPeriod(t *testing.T) {
	gin.SetMode(gin.TestMode)
	movements := stubMovements{rows: map[string][]movement.CategorySum{
		"type": {{Label: "income", Total: decimal.NewFromInt(84200)}},
	}}
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/overview"))

	body := w.Body.String()
	if !bodyContains(body, "Variaci") {
		t.Fatal("la variación debe mostrarse")
	}
	if bodyContains(body, "Sin movimientos") {
		t.Fatal("no puede decir 'sin movimientos' mientras muestra una variación de saldos")
	}
}

func sums(pairs ...any) []movement.CategorySum {
	var out []movement.CategorySum
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, movement.CategorySum{
			Label: pairs[i].(string),
			Total: decimal.NewFromInt(int64(pairs[i+1].(int))),
		})
	}
	return out
}

// SumForUser devuelve los buckets ordenados por total DESC. Sobre un eje de
// tiempo eso es ruido, así que buildTrendChart reconstruye la cronología — y
// las dos series tienen que caer en la misma columna, incluso en los buckets
// donde sólo una de ellas tiene fila.
func TestBuildTrendChart(t *testing.T) {
	tests := []struct {
		name              string
		expenses, incomes []movement.CategorySum
		display           func(string) string
		wantLabels        []string
		wantGastos        []float64
		wantIngresos      []float64
	}{
		{
			name:         "los días se ordenan cronológicamente, no por monto",
			expenses:     sums("2026-08-07", 300, "2026-08-03", 170, "2026-08-01", 60),
			incomes:      nil,
			display:      templates.ShortDay,
			wantLabels:   []string{"1", "3", "7"},
			wantGastos:   []float64{60, 170, 300},
			wantIngresos: []float64{0, 0, 0},
		},
		{
			name:         "un día con sólo ingresos igual abre columna de gastos",
			expenses:     sums("2026-08-02", 500),
			incomes:      sums("2026-08-09", 900, "2026-08-02", 100),
			display:      templates.ShortDay,
			wantLabels:   []string{"2", "9"},
			wantGastos:   []float64{500, 0},
			wantIngresos: []float64{100, 900},
		},
		{
			name:         "los meses salen con nombre corto",
			expenses:     sums("2026-08", 20, "2026-06", 90),
			incomes:      sums("2026-07", 40),
			display:      templates.ShortMonth,
			wantLabels:   []string{"jun", "jul", "ago"},
			wantGastos:   []float64{90, 0, 20},
			wantIngresos: []float64{0, 40, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildTrendChart(tt.expenses, tt.incomes, tt.display)

			if !reflect.DeepEqual(got.Labels, tt.wantLabels) {
				t.Errorf("labels = %v, want %v", got.Labels, tt.wantLabels)
			}
			if len(got.Datasets) != 2 {
				t.Fatalf("datasets = %d, want 2 (gastos e ingresos)", len(got.Datasets))
			}
			if got.Datasets[0].Label != "Gastos" || got.Datasets[1].Label != "Ingresos" {
				t.Fatalf("dataset labels = %q/%q", got.Datasets[0].Label, got.Datasets[1].Label)
			}
			if !reflect.DeepEqual(got.Datasets[0].Data, tt.wantGastos) {
				t.Errorf("gastos = %v, want %v", got.Datasets[0].Data, tt.wantGastos)
			}
			if !reflect.DeepEqual(got.Datasets[1].Data, tt.wantIngresos) {
				t.Errorf("ingresos = %v, want %v", got.Datasets[1].Data, tt.wantIngresos)
			}
		})
	}
}

// El caso reportado, mitad ida: Resumen es una vista de UN período y no ofrece
// rangos de tendencia, pero tiene que devolver intacto el slot que usan
// Evolución y el índice de Cuentas. Si no, cada paso por Resumen les borra la
// memoria.
func TestHandleOverview_AppStateKeepsTheTrendScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	movements := stubMovements{rows: map[string][]movement.CategorySum{
		"": {{Label: "", Total: decimal.NewFromInt(1234567)}},
	}}
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/overview?p=month&pt=3m"))

	body := w.Body.String()
	if !appStateHas(body, "p", "month") {
		t.Error("el Resumen debe publicar su propio preset en #app-state")
	}
	if !appStateHas(body, "pt", "3m") {
		t.Error("el Resumen borró el preset de las vistas de tendencia")
	}
}

// El dato que viaja al gráfico ya no lleva color: lleva un rol, y app.js lo
// resuelve leyendo el acento del tema. Es lo que paga la deuda de los dos
// azules que DESIGN.md dejó anotada — un hex acá es un azul que no sigue al
// usuario. Los colores de cuenta (AccountSlotColors) son la excepción y no
// entran en esta vista.
func TestOverview_TrendDataCarriesRolesNotColors(t *testing.T) {
	data := templates.OverviewData{
		TrendChart: templates.TrendChartData{
			Labels: []string{"jun", "jul"},
			Datasets: []templates.TrendDataset{
				{Label: "Gastos", Data: []float64{1, 2}, Role: templates.RoleExpense},
				{Label: "Ingresos", Data: []float64{3, 4}, Role: templates.RoleIncome},
			},
		},
	}

	var sb strings.Builder
	if err := templates.Overview(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if strings.Contains(html, "#2a78d6") || strings.Contains(html, "#1baf7a") {
		t.Errorf("el data island todavía manda un hex clavado; tiene que mandar el rol:\n%s", html)
	}
	if !strings.Contains(html, templates.RoleExpense) {
		t.Errorf("falta el rol %q en el data island:\n%s", templates.RoleExpense, html)
	}
	if !strings.Contains(html, templates.RoleIncome) {
		t.Errorf("falta el rol %q en el data island:\n%s", templates.RoleIncome, html)
	}
}
