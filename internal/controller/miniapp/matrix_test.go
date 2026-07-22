package miniapp

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/controller/miniapp/templates"
	"lopiibot.com/internal/movement"
)

type stubMovementsMatrix struct {
	stubMovementsWithAccounts
	byMonth map[string][]movement.CategorySum // keyed by "YYYY-MM" of q.From
	subs    map[string][]movement.CategorySum // keyed by category
}

func (s stubMovementsMatrix) SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error) {
	if groupBy == movement.GroupBySubcategory && q.Category != nil {
		return s.subs[*q.Category], nil
	}
	if groupBy == movement.GroupByCategory {
		return s.byMonth[q.From.Format("2006-01")], nil
	}
	return nil, nil
}

func d(v int64) decimal.Decimal { return decimal.NewFromInt(v) }

func TestBuildMatrixRows_SortsByPeriodTotalDesc(t *testing.T) {
	// El bug: iterar un map de Go randomiza el orden, así que dos cargas de la
	// misma data mostraban las filas distinto.
	rows := buildMatrixRows(
		[]string{"Transporte", "Vivienda", "Ocio"},
		map[string][]decimal.Decimal{
			"Transporte": {d(100), d(100)},
			"Vivienda":   {d(400), d(400)},
			"Ocio":       {d(50), d(300)},
		},
	)
	want := []string{"Vivienda", "Ocio", "Transporte"}
	for i, w := range want {
		if rows[i].Label != w {
			t.Errorf("rows[%d] = %q, want %q", i, rows[i].Label, w)
		}
	}
}

func TestBuildMatrixRows_ShadesAgainstRowAverage(t *testing.T) {
	rows := buildMatrixRows(
		[]string{"Ocio", "Vivienda", "Salud"},
		map[string][]decimal.Decimal{
			// promedio 125 → 200 es +60% (alto)
			"Ocio": {d(100), d(100), d(100), d(200)},
			// plano: nada se destaca aunque los números sean grandes
			"Vivienda": {d(400), d(400), d(400), d(400)},
			// promedio sobre meses CON movimiento (=30), no sobre 4
			"Salud": {d(0), d(30), d(0), d(30)},
		},
	)
	byLabel := map[string]templates.MatrixRow{}
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

func TestBuildMatrixRows_SingleMonthWithDataIsNeverShaded(t *testing.T) {
	// Sin al menos dos meses con movimiento no hay "normal" contra qué comparar.
	rows := buildMatrixRows([]string{"Viajes"}, map[string][]decimal.Decimal{
		"Viajes": {d(0), d(0), d(900000)},
	})
	for i, c := range rows[0].Cells {
		if c.Intensity != "" {
			t.Errorf("celda %d no debe sombrearse: %q", i, c.Intensity)
		}
	}
}

func TestHandleMatrix_RendersTableFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	current := templates.CurrentMonth(nowInART()).Format("2006-01")
	movements := stubMovementsMatrix{byMonth: map[string][]movement.CategorySum{
		current: {{Label: "Alimentación", Total: d(1500)}},
	}}
	c := NewController(movements, stubAccountsWithData{}, stubIcons{}, stubUsers{}, testBotToken)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/matrix?p=6m"))

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

func TestHandleMatrix_ExpandShowsSubcategories(t *testing.T) {
	gin.SetMode(gin.TestMode)
	current := templates.CurrentMonth(nowInART()).Format("2006-01")
	movements := stubMovementsMatrix{
		byMonth: map[string][]movement.CategorySum{
			current: {{Label: "Alimentación", Total: d(1500)}},
		},
		subs: map[string][]movement.CategorySum{
			"Alimentación": {{Label: "Supermercado", Total: d(1200)}},
		},
	}
	c := NewController(movements, stubAccountsWithData{}, stubIcons{}, stubUsers{}, testBotToken)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/matrix?p=6m&expand=Alimentaci%C3%B3n"))

	if !bodyContains(w.Body.String(), "Supermercado") {
		t.Fatal("expandir una categoría debe mostrar sus subcategorías")
	}
}
