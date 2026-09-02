package templates

import (
	"context"
	"strings"
	"testing"

	"lopiibot.com/internal/currency"
)

func TestOverview_TrendChartCarriesATextAlternative(t *testing.T) {
	data := OverviewData{
		Period: Period{Currency: currency.ARS},
		TrendChart: TrendChartData{
			Labels: []string{"jun", "jul", "ago"},
			Datasets: []TrendDataset{
				{Label: "Gastos", Data: []float64{100, 900, 300}},
				{Label: "Ingresos", Data: []float64{500, 500, 500}},
			},
		},
	}

	var sb strings.Builder
	if err := Overview(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if !strings.Contains(html, `id="resumen-trend" role="img" aria-label=`) {
		t.Fatalf("el grafico de Resumen no tiene alternativa textual, y es la unica representacion de la evolucion:\n%s", html)
	}
	for _, want := range []string{"Gastos", "Ingresos", "jul"} {
		if !strings.Contains(html, want) {
			t.Errorf("la alternativa textual no nombra %q:\n%s", want, html)
		}
	}
}

func TestAccounts_TrendChartCarriesATextAlternative(t *testing.T) {
	data := AccountsData{
		Period:    Period{Currency: currency.ARS, Months: 6},
		Snapshots: []AccountSnapshot{{Name: "Efectivo", Balance: "$1.000"}},
		TrendChart: TrendChartData{
			Labels:   []string{"jun", "jul"},
			Datasets: []TrendDataset{{Label: "Efectivo", Data: []float64{100, 200}}},
		},
	}

	var sb strings.Builder
	if err := Accounts(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if !strings.Contains(html, `id="cuentas-trend" role="img" aria-label=`) {
		t.Fatalf("el grafico de Cuentas no tiene alternativa textual:\n%s", html)
	}
}

func TestCategories_ChartDefersToTheTable(t *testing.T) {
	data := CategoriesData{
		Period: Period{Currency: currency.ARS},
		Rows: []CategoryRow{
			{Category: "Alimentación", Total: "$100", PerDay: "$3", Href: "/app/categories?p=month&c=Alimentaci%C3%B3n"},
			{Category: "Transporte", Total: "$60", PerDay: "$2"},
		},
		Total: "$160",
		Chart: BarChartData{Labels: []string{"Alimentación", "Transporte"}, Values: []float64{100, 60}},
	}

	var sb strings.Builder
	if err := Categories(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if !strings.Contains(html, `id="categorias-bars" aria-hidden="true"`) {
		t.Errorf("el grafico repite la tabla de abajo y no esta marcado como decorativo, asi que se lee dos veces:\n%s", html)
	}
	if !strings.Contains(html, `aria-label="Gastos por categoría"`) {
		t.Errorf("la tabla no tiene nombre accesible: navegando por tablas se anuncia como \"tabla\" a secas:\n%s", html)
	}
	if got := strings.Count(html, `<th scope="row">`); got != len(data.Rows) {
		t.Errorf("encabezados de fila = %d, want %d: sin th la celda de monto no puede nombrar su categoria", got, len(data.Rows))
	}
}

func TestAccountLeaf_FilterMessageIsALiveRegion(t *testing.T) {
	data := AccountLeafData{
		Period:      Period{Currency: currency.ARS},
		AccountName: "Efectivo",
		Rows:        []MovementRow{{Title: "Súper", Amount: "$100", Date: "2026-08-29"}},
	}

	var sb strings.Builder
	if err := AccountLeaf(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	i := strings.Index(html, `id="`+MovFilterEmptyID+`"`)
	if i < 0 {
		t.Fatalf("no esta el cartel de filtro vacio:\n%s", html)
	}
	region := html[i:]
	if j := strings.Index(region, ">"); j >= 0 {
		region = region[:j]
	}
	if !strings.Contains(region, `role="status"`) {
		t.Errorf("el cartel no es live region, asi que filtrar a cero no se anuncia: %s", region)
	}
	if !strings.Contains(region, `data-msg=`) {
		t.Errorf("el mensaje no viaja en data-msg: %s", region)
	}
	if strings.Contains(html, ">"+MsgFilterNoMatch+"<") {
		t.Errorf("el mensaje viene renderizado: una live region tiene que arrancar vacia para que insertar el texto se anuncie:\n%s", html)
	}
}

func TestShell_CarriesTheRouteStatusRegion(t *testing.T) {
	var sb strings.Builder
	if err := Shell(TabOverview, RouteOverview).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if !strings.Contains(html, `id="`+RouteStatusID+`"`) {
		t.Fatalf("el shell no tiene region de estado: cada swap de htmx queda mudo:\n%s", html)
	}
	status := html[strings.Index(html, `id="`+RouteStatusID+`"`):]
	status = status[:strings.Index(status, ">")]
	if !strings.Contains(status, `role="status"`) {
		t.Errorf("la region no es live: %s", status)
	}
	if strings.Index(html, RouteStatusID) > strings.Index(html, `id="content"`) {
		t.Error("la region esta adentro o despues de #content, asi que el swap la reemplaza y deja de anunciar")
	}
}

func TestTrendChartAlt_NamesEachSeriesAndItsPeak(t *testing.T) {
	got := TrendChartAlt("gastos e ingresos", TrendChartData{
		Labels: []string{"jun", "jul", "ago"},
		Datasets: []TrendDataset{
			{Label: "Gastos", Data: []float64{100, 900, 300}},
		},
	}, currency.ARS)

	for _, want := range []string{"gastos e ingresos", "de jun a ago", "Gastos:", "máximo", "en jul"} {
		if !strings.Contains(got, want) {
			t.Errorf("la alternativa no dice %q: %s", want, got)
		}
	}
}

func TestTrendChartAlt_SurvivesEmptyAndSinglePointSeries(t *testing.T) {
	if got := TrendChartAlt("saldo por cuenta", TrendChartData{}, currency.ARS); !strings.Contains(got, "sin datos") {
		t.Errorf("sin datos = %q", got)
	}

	got := TrendChartAlt("saldo por cuenta", TrendChartData{
		Labels:   []string{"ago"},
		Datasets: []TrendDataset{{Label: "Efectivo", Data: []float64{100}}},
	}, currency.ARS)
	if strings.Contains(got, "máximo") {
		t.Errorf("un solo punto no se describe como un rango con pico: %q", got)
	}
	if !strings.Contains(got, "en ago") {
		t.Errorf("no ubica el unico punto: %q", got)
	}
	if !strings.Contains(got, "Efectivo") {
		t.Errorf("no nombra la serie: %q", got)
	}
}

func TestOverview_HasAHeadingEvenThoughItShowsNone(t *testing.T) {
	var sb strings.Builder
	if err := Overview(OverviewData{Period: Period{Currency: currency.ARS}, Empty: true}).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if !strings.Contains(html, `<h1 class="sr-only">`) {
		t.Errorf("la pantalla de entrada no tiene ningun encabezado, asi que navegando por encabezados no hay donde anclar:\n%s", html)
	}
}

func TestEvolution_LegendSwatchesAreDecorative(t *testing.T) {
	data := EvolutionData{
		Period: Period{Currency: currency.ARS},
		Months: []string{"2026-08"},
		Rows:   []EvolutionRow{{Label: "Alimentación", Cells: []EvolutionCell{{Value: "60"}}}},
		Totals: []EvolutionCell{{Value: "60"}},
	}

	var sb strings.Builder
	if err := Evolution(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if got := strings.Count(html, `legend-swatch`); got != 2 {
		t.Fatalf("muestras = %d, want 2", got)
	}
	if got := strings.Count(html, `aria-hidden="true"></span>`); got != 2 {
		t.Errorf("las muestras son spans vacios y no estan marcadas como decorativas:\n%s", html)
	}
}

func TestPeriodHeader_NamesTheChipGroup(t *testing.T) {
	var sb strings.Builder
	if err := PeriodHeader(Period{Currency: currency.ARS}).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	if html := sb.String(); !strings.Contains(html, `role="group" aria-label=`) {
		t.Errorf("el grupo de chips no tiene nombre accesible:\n%s", html)
	}
}
