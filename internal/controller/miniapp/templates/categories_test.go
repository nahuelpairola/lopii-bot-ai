package templates

import (
	"context"
	"strings"
	"testing"
)

func TestSubcategoryDrill_RowsLinkToTheirLeaf(t *testing.T) {
	data := CategoriesData{
		Drill: "Alimentación",
		Rows: []CategoryRow{
			{Category: "Supermercado", Total: "$80.000", Share: "65%", Href: "/app/categories?p=month&m=2026-08&c=ARS&category=Alimentaci%C3%B3n&subcategory=Supermercado"},
		},
		Total: "$123.000",
	}

	var sb strings.Builder
	if err := SubcategoryDrill(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if !strings.Contains(html, "subcategory=Supermercado") {
		t.Errorf("una subcategoría ahora SÍ tiene algo abajo: los movimientos que la componen\n%s", html)
	}
	if !strings.Contains(html, `hx-push-url="true"`) {
		t.Errorf("sin push-url el botón Atrás de Telegram no vuelve:\n%s", html)
	}
}

func TestSubcategoryLeaf_ShowsMovementsAndRealTotal(t *testing.T) {
	data := SubcategoryLeafData{
		Category:    "Alimentación",
		Subcategory: "Supermercado",
		BackQuery:   "/app/categories?p=month&m=2026-08&c=ARS&category=Alimentaci%C3%B3n",
		Total:       "$80.000",
		Rows: []MovementRow{
			{Title: "Coto", Date: "12 ago", Amount: "$34.500"},
			{Title: "Dia", Date: "11 ago", Amount: "$12.000"},
		},
	}

	var sb strings.Builder
	if err := SubcategoryLeaf(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	for _, want := range []string{"Supermercado", "Coto", "12 ago", "$34.500", "Dia", "$80.000"} {
		if !strings.Contains(html, want) {
			t.Errorf("falta %q en la hoja:\n%s", want, html)
		}
	}
	// El monto va en positivo: son todos gastos y el signo no aporta nada. Es la
	// diferencia deliberada con la hoja de cuenta, donde la dirección ES el dato.
	if strings.Contains(html, "-$34.500") {
		t.Error("en esta hoja el monto va sin signo: son todos gastos")
	}
	if strings.Contains(html, "<table") {
		t.Error("la hoja es una lista, no una tabla")
	}
}

func TestSubcategoryLeaf_Empty(t *testing.T) {
	data := SubcategoryLeafData{Category: "Alimentación", Subcategory: "Supermercado", Empty: true}

	var sb strings.Builder
	if err := SubcategoryLeaf(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(sb.String(), "Sin gastos en esta subcategoría.") {
		t.Errorf("falta el estado vacío:\n%s", sb.String())
	}
}

// Las dos pantallas profundas de Categorias tenian el Volver ABAJO del titulo.
// Suben, y toman .tappable como el de la hoja de cuenta.
func TestCategoriesDeepViews_BackLinkSitsAboveTheTitle(t *testing.T) {
	cases := []struct {
		name   string
		render func() (string, error)
	}{
		{"drill", func() (string, error) {
			var sb strings.Builder
			err := SubcategoryDrill(CategoriesData{Drill: "Alimentación"}).Render(context.Background(), &sb)
			return sb.String(), err
		}},
		{"hoja", func() (string, error) {
			var sb strings.Builder
			err := SubcategoryLeaf(SubcategoryLeafData{
				Category: "Alimentación", Subcategory: "Supermercado", BackQuery: "/app/categories?p=month",
			}).Render(context.Background(), &sb)
			return sb.String(), err
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			html, err := c.render()
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if got := strings.Count(html, "<h1>"); got != 1 {
				t.Errorf("h1 = %d, want 1:\n%s", got, html)
			}
			if strings.Contains(html, "<h2>") {
				t.Errorf("quedo un h2:\n%s", html)
			}
			if strings.Index(html, "Volver") > strings.Index(html, "<h1>") {
				t.Errorf("el Volver quedo abajo del titulo:\n%s", html)
			}
			if !strings.Contains(html, "tappable") {
				t.Errorf("el Volver no toma .tappable:\n%s", html)
			}
		})
	}
}

// El grafico de Categorias es de barras HORIZONTALES: cada barra es una
// categoria, y con el alto fijo de Chart.js veinte categorias son veinte
// pelitos. Necesita el envoltorio para que app.js le pueda dar un alto que
// crece con las filas.
func TestCategories_ChartSitsInABox(t *testing.T) {
	data := CategoriesData{
		Rows:  []CategoryRow{{Category: "Alimentación", Total: "$1"}},
		Chart: BarChartData{Labels: []string{"Alimentación"}, Values: []float64{1}},
		Total: "$1",
	}

	var sb strings.Builder
	if err := Categories(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if !strings.Contains(html, `class="chart-box"`) {
		t.Errorf("el canvas no esta adentro de .chart-box:\n%s", html)
	}
	if strings.Index(html, `class="chart-box"`) > strings.Index(html, "<canvas") {
		t.Errorf("el envoltorio tiene que CONTENER al canvas, no venir despues:\n%s", html)
	}
}
