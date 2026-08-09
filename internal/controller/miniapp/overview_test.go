package miniapp

import (
	"net/http/httptest"
	"reflect"
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
	// Con ventana de un mes el trend agrupa por día, y lleva las dos series
	// igual que las ventanas más largas: un mes sin la columna de ingresos no
	// deja comparar contra lo que entró.
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
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken)
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
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken)
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
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken)
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
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken)
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
