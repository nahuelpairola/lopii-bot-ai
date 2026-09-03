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
			{Category: "Supermercado", Total: "$80.000", PerDay: "$2.580", Href: "/app/categories?p=month&m=2026-08&c=ARS&category=Alimentaci%C3%B3n&subcategory=Supermercado"},
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

func TestCategories_TotalSitsAboveTheTable(t *testing.T) {
	data := CategoriesData{
		Rows: []CategoryRow{
			{Category: "Alimentación", Total: "$60.000", PerDay: "$1.935"},
			{Category: "Transporte", Total: "$20.000", PerDay: "$645"},
		},
		Chart: BarChartData{Labels: []string{"Alimentación", "Transporte"}, Values: []float64{60000, 20000}},
		Total: "$80.000",
	}

	var sb strings.Builder
	if err := Categories(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if strings.Index(html, "$80.000") > strings.Index(html, "<table") {
		t.Errorf("el total sigue abajo de la tabla:\n%s", html)
	}
	if got := strings.Count(html, "$80.000"); got != 1 {
		t.Errorf("el total aparece %d veces, want 1 — no se duplica arriba y en el pie:\n%s", got, html)
	}
}

func TestCategoriesDrill_HeaderIsOneTotalLine(t *testing.T) {
	data := CategoriesData{
		Drill: "Alimentación",
		Rows:  []CategoryRow{{Category: "Supermercado", Total: "$412.300", PerDay: "$13.300"}},
		Total: "$412.300",
	}

	var sb strings.Builder
	if err := SubcategoryDrill(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	back := strings.Index(html, "Volver")
	h1 := strings.Index(html, "<h1>")
	total := strings.Index(html, "page-total")
	if total < 0 {
		t.Fatalf("el total no toma .page-total, asi que sigue siendo un bloque aparte:\n%s", html)
	}
	if !(back < h1 && h1 < total) {
		t.Errorf("el orden tiene que ser Volver → h1 → total (got %d, %d, %d):\n%s", back, h1, total, html)
	}
	if strings.Contains(html[h1:total], `class="group-title"`) {
		t.Errorf("el rotulo Total sigue en su propio renglon arriba de la cifra:\n%s", html)
	}
	if !strings.Contains(html, `class="balance-line page-total"`) {
		t.Errorf("el total no toma la forma que ya usan las dos hojas de movimientos:\n%s", html)
	}

	if !strings.Contains(html, `<span class="money">$412.300</span>`) {
		t.Errorf("la cifra perdio .money, y con eso los numerales tabulares:\n%s", html)
	}
}

func TestCategories_TableShowsShareAndPerDay(t *testing.T) {
	data := CategoriesData{
		Rows: []CategoryRow{
			{Category: "Alimentación", Total: "$60.000", Share: "75%", PerDay: "$1.935"},
		},
		Total:      "$60.000",
		PerDayNote: "Por día = total ÷ 31 días corridos, del 1 al 31 jul.",
	}

	var sb strings.Builder
	if err := Categories(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if !strings.Contains(html, "$1.935") {
		t.Errorf("la fila no muestra su costo por día:\n%s", html)
	}
	if !strings.Contains(html, `<th scope="col">Por día</th>`) {
		t.Errorf("la columna no está encabezada:\n%s", html)
	}
	if !strings.Contains(html, `<th scope="col">%</th>`) {
		t.Errorf("el porcentaje volvió a la tabla, entre Total y Por día:\n%s", html)
	}
	if !strings.Contains(html, "75%") {
		t.Errorf("la fila no muestra su participación en el total:\n%s", html)
	}
	if strings.Index(html, `<th scope="col">%</th>`) > strings.Index(html, `<th scope="col">Por día</th>`) {
		t.Errorf("el porcentaje va antes de Por día, no después:\n%s", html)
	}
	if !strings.Contains(html, data.PerDayNote) {
		t.Errorf("sin la nota, el mismo número significa algo distinto en cada pestaña y nada lo aclara:\n%s", html)
	}
	if strings.Index(html, "<table") > strings.Index(html, data.PerDayNote) {
		t.Errorf("la nota explica la tabla, así que va debajo de ella:\n%s", html)
	}
}

func TestSubcategoryDrill_CarriesThePerDayColumnToo(t *testing.T) {
	data := CategoriesData{
		Drill:      "Alimentación",
		Rows:       []CategoryRow{{Category: "Supermercado", Total: "$412.300", PerDay: "$13.300"}},
		Total:      "$412.300",
		PerDayNote: "Por día = total ÷ 31 días corridos, del 1 al 31 jul.",
	}

	var sb strings.Builder
	if err := SubcategoryDrill(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if !strings.Contains(html, "$13.300") {
		t.Errorf("el drill comparte categoryTable, así que la subcategoría también lleva su por día:\n%s", html)
	}
	if !strings.Contains(html, data.PerDayNote) {
		t.Errorf("la nota vive dentro de categoryTable y el drill la hereda:\n%s", html)
	}
}
