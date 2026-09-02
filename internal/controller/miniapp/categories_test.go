package miniapp

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

type stubIcons struct{}

func (stubIcons) IconForCategory(_ uint64, category string) string {
	if category == "Alimentación" {
		return "🍔"
	}
	return "📂"
}

func TestHandleCategories_RendersIcons(t *testing.T) {
	gin.SetMode(gin.TestMode)
	movements := stubMovements{rows: map[string][]movement.CategorySum{
		"category": {{Label: "Alimentación", Total: decimal.NewFromInt(5000)}},
	}}
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/categories"))

	if !bodyContains(w.Body.String(), "🍔") {
		t.Fatal("cada categoría debe llevar su icono")
	}
}

type recordingMovements struct {
	stubMovements
	queries *[]movement.MovementQuery
}

func (s recordingMovements) SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error) {
	*s.queries = append(*s.queries, q)
	return s.stubMovements.SumForUser(q, groupBy)
}

func TestHandleCategories_AsksWithoutReservedCategories(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var queries []movement.MovementQuery
	movements := recordingMovements{
		stubMovements: stubMovements{rows: map[string][]movement.CategorySum{
			"category": {{Label: "Alimentación", Total: decimal.NewFromInt(5000)}},
		}},
		queries: &queries,
	}
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/categories"))

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(queries) == 0 {
		t.Fatal("la vista no consultó movimientos")
	}
	for _, q := range queries {
		if q.OnlyReserved {
			t.Fatal("Categorías no debe pedir las reservadas: son plomería, no gasto del usuario")
		}
	}
	if !bodyContains(w.Body.String(), "Alimentaci") {
		t.Fatal("expected the real category to render")
	}
}

func TestHandleCategoryDrill_CategoryWithSlash(t *testing.T) {
	gin.SetMode(gin.TestMode)

	movements := stubMovements{rows: map[string][]movement.CategorySum{
		"subcategory": {{Label: "Cuota préstamo", Total: decimal.NewFromInt(8000)}},
	}}
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/categories?category=Deudas+%2F+pr%C3%A9stamos"))

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !bodyContains(w.Body.String(), "Cuota") {
		t.Fatal("el drill debe mostrar las subcategorías de la categoría pedida")
	}
}

func TestHandleCategories_ShowsWhatEachCategoryCostsPerDay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	movements := stubMovements{rows: map[string][]movement.CategorySum{
		"category": {
			{Label: "Alimentación", Total: decimal.NewFromInt(31000)},
			{Label: "Transporte", Total: decimal.NewFromInt(6200)},
		},
	}}
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/categories?p=month&m=2026-07&c=ARS"))

	body := w.Body.String()
	if !bodyContains(body, "$1.000") {
		t.Fatal("$31.000 en julio son $1.000 por día: 31 días corridos")
	}
	if !bodyContains(body, "$200") {
		t.Fatal("$6.200 en julio son $200 por día")
	}
	if !bodyContains(body, "31 días corridos") {
		t.Fatal("la nota debe nombrar el divisor que se usó")
	}
	if bodyContains(body, `<th scope="col">%</th>`) {
		t.Fatal("el porcentaje salió de la tabla")
	}
}

func bodyContains(body, substr string) bool {
	return len(body) > 0 && (func() bool {
		for i := 0; i+len(substr) <= len(body); i++ {
			if body[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}

type stubMovementsWithRows struct {
	stubMovements
	movements []movement.Movement

	lastQuery *movement.MovementQuery
}

func (s stubMovementsWithRows) ListForUser(q movement.MovementQuery, limit int) ([]movement.Movement, error) {
	*s.lastQuery = q
	return s.movements, nil
}

func TestHandleSubcategoryLeaf_ListsMovementsOfThatSubcategory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	desc := "Coto"
	var got movement.MovementQuery
	movements := stubMovementsWithRows{
		stubMovements: stubMovements{rows: map[string][]movement.CategorySum{

			"": {{Label: "", Total: decimal.NewFromInt(80000)}},
		}},
		movements: []movement.Movement{{
			Date: time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC),
			Type: movement.Expense, Amount: decimal.NewFromInt(-34500), Currency: currency.ARS,
			Description: &desc,
		}},
		lastQuery: &got,
	}
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/categories?category=Alimentaci%C3%B3n&subcategory=Supermercado"))
	body := w.Body.String()

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, body)
	}
	if !bodyContains(body, "Coto") {
		t.Errorf("la hoja tiene que listar los movimientos de la subcategoría:\n%s", body)
	}
	if !bodyContains(body, "$34.500") {
		t.Errorf("el monto va en positivo, son todos gastos:\n%s", body)
	}
	if !bodyContains(body, "$80.000") {
		t.Errorf("el total del pie es el mismo número de la fila que abrió la hoja:\n%s", body)
	}
	if got.Subcategory == nil || *got.Subcategory != "Supermercado" {
		t.Errorf("la lista se tiene que pedir filtrada por subcategoría, got %v", got.Subcategory)
	}
	if got.Category == nil || *got.Category != "Alimentación" {
		t.Errorf("y también por categoría: dos categorías pueden tener la misma subcategoría, got %v", got.Category)
	}
}

func TestHandleCategories_PeriodChipsKeepTheDrill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	movements := stubMovements{rows: map[string][]movement.CategorySum{
		"category":    {{Label: "Alimentación", Total: decimal.NewFromInt(5000)}},
		"subcategory": {{Label: "Supermercado", Total: decimal.NewFromInt(5000)}},
	}}
	c := NewController(movements, stubAccounts{}, stubIcons{}, stubUsers{}, testBotToken, testBotUsername)
	router := gin.New()
	c.RegisterRoutes(router)

	cases := []struct{ url, want string }{
		{"/app/categories?category=Alimentaci%C3%B3n&p=month&m=2026-07", "&category=Alimentaci%C3%B3n"},
		{"/app/categories?category=Alimentaci%C3%B3n&subcategory=Supermercado&p=month&m=2026-07", "&category=Alimentaci%C3%B3n&subcategory=Supermercado"},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, authedHTMXRequest(t, tc.url))
		if !leafKeepsDrill(w.Body.String(), tc.want) {
			t.Errorf("%s: cambiar el rango pierde %s y vuelve al índice", tc.url, tc.want)
		}
	}
}
