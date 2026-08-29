package miniapp

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/controller/miniapp/templates"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

type stubMovementsEvolution struct {
	stubMovementsWithAccounts
	byMonth map[string][]movement.CategorySum
	subs    map[string][]movement.CategorySum
}

func (s stubMovementsEvolution) SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error) {
	if groupBy == movement.GroupBySubcategory && q.Category != nil {
		return s.subs[*q.Category], nil
	}
	if groupBy == movement.GroupByCategory {
		return s.byMonth[q.From.Format("2006-01")], nil
	}
	return nil, nil
}

func d(v int64) decimal.Decimal { return decimal.NewFromInt(v) }

func TestBuildEvolutionRows_SortsByPeriodTotalDesc(t *testing.T) {

	rows := buildEvolutionRows(
		[]string{"Transporte", "Vivienda", "Ocio"},
		map[string][]decimal.Decimal{
			"Transporte": {d(100), d(100)},
			"Vivienda":   {d(400), d(400)},
			"Ocio":       {d(50), d(300)},
		},
		currency.ARS,
	)
	want := []string{"Vivienda", "Ocio", "Transporte"}
	for i, w := range want {
		if rows[i].Label != w {
			t.Errorf("rows[%d] = %q, want %q", i, rows[i].Label, w)
		}
	}
}

func TestBuildEvolutionRows_ShadesAgainstRowAverage(t *testing.T) {
	rows := buildEvolutionRows(
		[]string{"Ocio", "Vivienda", "Salud"},
		map[string][]decimal.Decimal{

			"Ocio": {d(100), d(100), d(100), d(200)},

			"Vivienda": {d(400), d(400), d(400), d(400)},

			"Salud": {d(0), d(30), d(0), d(30)},
		},
		currency.ARS,
	)
	byLabel := map[string]templates.EvolutionRow{}
	for _, r := range rows {
		byLabel[r.Label] = r
	}

	if got := byLabel["Ocio"].Cells[3].Intensity; got != templates.CellHigh {
		t.Errorf("Ocio último mes: Intensity = %q, want %q", got, templates.CellHigh)
	}
	if got := byLabel["Ocio"].Cells[0].Intensity; got != "" {
		t.Errorf("Ocio por debajo de su promedio: Intensity = %q, want vacío", got)
	}
	for i, c := range byLabel["Vivienda"].Cells {
		if c.Intensity != "" {
			t.Errorf("Vivienda es plana, celda %d no debe sombrearse (got %q)", i, c.Intensity)
		}
	}
	for i, c := range byLabel["Salud"].Cells {
		if c.Intensity != "" {
			t.Errorf("Salud está en su promedio, celda %d no debe sombrearse (got %q)", i, c.Intensity)
		}
	}
	if byLabel["Salud"].Cells[0].Value != "·" {
		t.Errorf("un mes sin movimiento se renderiza como punto, got %q", byLabel["Salud"].Cells[0].Value)
	}
}

func TestBuildEvolutionRows_SingleMonthWithDataIsNeverShaded(t *testing.T) {

	rows := buildEvolutionRows([]string{"Viajes"}, map[string][]decimal.Decimal{
		"Viajes": {d(0), d(0), d(900000)},
	}, currency.ARS)
	for i, c := range rows[0].Cells {
		if c.Intensity != "" {
			t.Errorf("celda %d no debe sombrearse: %q", i, c.Intensity)
		}
	}
}

func TestHandleEvolution_RendersTableFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	current := templates.CurrentMonth(nowInART()).Format("2006-01")
	movements := stubMovementsEvolution{byMonth: map[string][]movement.CategorySum{
		current: {{Label: "Alimentación", Total: d(1500)}},
	}}
	c := NewController(movements, stubAccountsWithData{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/evolution?p=6m"))

	body := w.Body.String()
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, body)
	}
	if !bodyContains(body, "<table") {
		t.Fatal("la matriz mantiene su <table> semántico")
	}
	if !bodyContains(body, "Alimentación") {
		t.Fatal("las filas son categorías, no subcategorías")
	}
}

func TestHandleEvolution_ExpandShowsSubcategories(t *testing.T) {
	gin.SetMode(gin.TestMode)
	current := templates.CurrentMonth(nowInART()).Format("2006-01")
	movements := stubMovementsEvolution{
		byMonth: map[string][]movement.CategorySum{
			current: {{Label: "Alimentación", Total: d(1500)}},
		},
		subs: map[string][]movement.CategorySum{
			"Alimentación": {{Label: "Supermercado", Total: d(1200)}},
		},
	}
	c := NewController(movements, stubAccountsWithData{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/evolution?p=6m&expand=Alimentaci%C3%B3n"))

	if !bodyContains(w.Body.String(), "Supermercado") {
		t.Fatal("expandir una categoría debe mostrar sus subcategorías")
	}
}

func TestHandleEvolution_AppStateKeepsTheSinglePeriodScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	current := templates.CurrentMonth(nowInART()).Format("2006-01")
	movements := stubMovementsEvolution{byMonth: map[string][]movement.CategorySum{
		current: {{Label: "Alimentación", Total: d(1500)}},
	}}
	c := NewController(movements, stubAccountsWithData{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/evolution?p=month&pt=6m"))

	body := w.Body.String()
	if !appStateHas(body, "pt", "6m") {
		t.Error("Evolución debe publicar su propio preset en #app-state")
	}
	if !appStateHas(body, "p", "month") {
		t.Error("Evolución pisó el preset de las vistas de un período: ése es el bug")
	}
}
