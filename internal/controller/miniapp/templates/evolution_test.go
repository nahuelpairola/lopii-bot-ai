package templates

import (
	"context"
	"strings"
	"testing"

	"lopiibot.com/internal/currency"
)

// La leyenda explica lo que estas por leer, no lo que ya leiste: estaba DEBAJO
// de la tabla entera, asi que habia que scrollear pasando el sombreado para
// enterarse de que era. Y nombra los DOS pasos: cellIntensity grada contra el
// promedio de la propia fila con dos umbrales, no con uno.
func TestEvolution_ExplainsBothShadingsAboveTheTable(t *testing.T) {
	data := EvolutionData{
		Period: Period{Currency: currency.ARS},
		Months: []string{"2026-07", "2026-08"},
		Rows: []EvolutionRow{{
			Label: "Alimentación",
			Cells: []EvolutionCell{{Value: "60"}, {Value: "20", Intensity: CellHigh}},
		}},
		Totals: []EvolutionCell{{Value: "60"}, {Value: "20"}},
	}

	var sb strings.Builder
	if err := Evolution(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	legend := strings.Index(html, "legend-swatch")
	table := strings.Index(html, "<table")
	if legend < 0 {
		t.Fatalf("no hay leyenda con muestras de color:\n%s", html)
	}
	if legend > table {
		t.Errorf("la leyenda sigue abajo de la tabla:\n%s", html)
	}
	for _, class := range []string{CellMild, CellHigh} {
		if !strings.Contains(html, "legend-swatch "+class) {
			t.Errorf("la leyenda no muestra el paso %q, y el sombreado tiene dos:\n%s", class, html)
		}
	}
	if !strings.Contains(html, "en miles de $") {
		t.Errorf("la escala se perdio al salir del titulo:\n%s", html)
	}
	if strings.Contains(html, "Gasto por categoría ·") {
		t.Errorf("el titulo se quedo con la escala adentro:\n%s", html)
	}
}

// USD no se escala, asi que la leyenda no puede emitir el segmento ni el
// separador que lo precede: con el separador adentro de la etiqueta —como
// estaba— quedaba un "·" colgado.
func TestEvolution_DropsTheScaleSegmentForUSD(t *testing.T) {
	data := EvolutionData{
		Period: Period{Currency: currency.USD},
		Months: []string{"2026-08"},
		Rows:   []EvolutionRow{{Label: "Ocio", Cells: []EvolutionCell{{Value: "20"}}}},
		Totals: []EvolutionCell{{Value: "20"}},
	}

	var sb strings.Builder
	if err := Evolution(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	if html := sb.String(); strings.Contains(html, "en miles de") {
		t.Errorf("USD no se escala y la leyenda igual lo dice:\n%s", html)
	}
}
