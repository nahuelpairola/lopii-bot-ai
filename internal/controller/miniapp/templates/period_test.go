package templates

import (
	"strings"
	"testing"
	"time"

	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/currency"
)

func art(y int, m time.Month) time.Time {
	return time.Date(y, m, 1, 0, 0, 0, 0, constants.ArgentinaZone)
}

func TestCurrentMonth_UsesArgentineWallClock(t *testing.T) {
	// 01:30 UTC del 1/8 = 22:30 ART del 31/7 → el mes corriente es julio.
	got := CurrentMonth(time.Date(2026, 8, 1, 1, 30, 0, 0, time.UTC))
	if !got.Equal(art(2026, time.July)) {
		t.Fatalf("got %s, want 2026-07 ART", got.Format("2006-01 MST"))
	}
}

func TestNewPeriod_MonthWindow(t *testing.T) {
	p := NewPeriod(RouteOverview, PresetMonth, art(2026, time.July), art(2026, time.July), currency.ARS, AllPresets)

	if p.Months != 1 {
		t.Errorf("Months = %d, want 1", p.Months)
	}
	if !p.From.Equal(art(2026, time.July)) {
		t.Errorf("From = %s, want 2026-07-01", p.From)
	}
	if p.To.Format("2006-01-02") != "2026-07-31" {
		t.Errorf("To = %s, want 2026-07-31", p.To.Format("2006-01-02"))
	}
	if p.Label != "Julio 2026" {
		t.Errorf("Label = %q, want %q", p.Label, "Julio 2026")
	}
	if p.NextQuery != "" {
		t.Errorf("NextQuery = %q, want empty — no se navega al futuro", p.NextQuery)
	}
}

func TestNewPeriod_SixMonthWindowEndingAtAnchor(t *testing.T) {
	p := NewPeriod(RouteEvolution, Preset6M, art(2026, time.July), art(2026, time.July), currency.ARS, TrendPresets)

	if !p.From.Equal(art(2026, time.February)) {
		t.Errorf("From = %s, want 2026-02 (6 meses terminando en julio)", p.From)
	}
	if p.Label != "Feb – jul 2026" {
		t.Errorf("Label = %q, want %q", p.Label, "Feb – jul 2026")
	}
	want := []string{"2026-02", "2026-03", "2026-04", "2026-05", "2026-06", "2026-07"}
	got := p.MonthKeys()
	if len(got) != len(want) {
		t.Fatalf("MonthKeys len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("MonthKeys[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestNewPeriod_CursorStepsByWindowLength(t *testing.T) {
	// Ancla en enero 2026, ventana de 3 meses, mes corriente julio 2026.
	p := NewPeriod(RouteEvolution, Preset3M, art(2026, time.January), art(2026, time.July), currency.ARS, TrendPresets)

	if p.Label != "Nov 2025 – ene 2026" {
		t.Errorf("Label = %q, want %q", p.Label, "Nov 2025 – ene 2026")
	}
	if p.PrevQuery != "/app/evolution?c=ARS&m=2025-10&p=3m" {
		t.Errorf("PrevQuery = %q", p.PrevQuery)
	}
	if p.NextQuery != "/app/evolution?c=ARS&m=2026-04&p=3m" {
		t.Errorf("NextQuery = %q", p.NextQuery)
	}
}

func TestNewPeriod_NextClampedAtCurrentMonth(t *testing.T) {
	// Ancla mayo, ventana 3m, mes corriente julio: el próximo salto sería
	// agosto, que es futuro → sin NextQuery.
	p := NewPeriod(RouteEvolution, Preset3M, art(2026, time.May), art(2026, time.July), currency.ARS, TrendPresets)
	if p.NextQuery != "" {
		t.Errorf("NextQuery = %q, want empty", p.NextQuery)
	}
}

func TestPeriod_LinkBuildersAreAbsolute(t *testing.T) {
	// Los links salen con la ruta adelante: una query suelta ("?p=3m") la
	// resolvería htmx contra la URL actual, que en el drill lleva params
	// ajenos.
	p := NewPeriod(RouteAccounts, Preset6M, art(2026, time.July), art(2026, time.July), currency.ARS, TrendPresets)

	if got := p.WithPreset(Preset3M); got != "/app/accounts?c=ARS&m=2026-07&p=3m" {
		t.Errorf("WithPreset = %q", got)
	}
	if got := p.WithCurrency(currency.USD); got != "/app/accounts?c=USD&m=2026-07&p=6m" {
		t.Errorf("WithCurrency = %q", got)
	}
	if got := p.Query(); got != "/app/accounts?c=ARS&m=2026-07&p=6m" {
		t.Errorf("Query = %q", got)
	}
	if !p.IsPreset(Preset6M) || p.IsPreset(Preset3M) {
		t.Error("IsPreset debe marcar solo el preset activo")
	}
}

func TestWithDrill_HeaderLinksKeepTheLeaf(t *testing.T) {
	// Ancla en junio con julio corriente: así el período tiene flecha para los
	// dos lados y se prueban las cuatro salidas de la cabecera.
	p := NewPeriod(RouteAccounts, PresetMonth, art(2026, time.June), art(2026, time.July), currency.ARS, AllPresets).
		WithDrill("&account=12")

	links := map[string]string{
		"PrevQuery":    p.PrevQuery,
		"NextQuery":    p.NextQuery,
		"WithPreset":   p.WithPreset(Preset6M),
		"WithCurrency": p.WithCurrency(currency.USD),
	}
	for name, link := range links {
		if !strings.Contains(link, "&account=12") {
			t.Errorf("%s = %q, le falta el drill: cambiar el rango sale de la hoja", name, link)
		}
	}

	// Query() es el link de VUELTA al índice: si arrastrara el drill, "Volver a
	// Cuentas" te dejaría en la misma hoja.
	if strings.Contains(p.Query(), "account=") {
		t.Errorf("Query() = %q, no debería llevar el drill", p.Query())
	}
}
