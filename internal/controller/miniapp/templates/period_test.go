package templates

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/currency"
)

func art(y int, m time.Month) time.Time {
	return time.Date(y, m, 1, 0, 0, 0, 0, constants.ArgentinaZone)
}

func presetsFor(single, trend string) map[string]string {
	return map[string]string{SinglePeriodScope.Param: single, TrendScope.Param: trend}
}

func TestCurrentMonth_UsesArgentineWallClock(t *testing.T) {

	got := CurrentMonth(time.Date(2026, 8, 1, 1, 30, 0, 0, time.UTC))
	if !got.Equal(art(2026, time.July)) {
		t.Fatalf("got %s, want 2026-07 ART", got.Format("2006-01 MST"))
	}
}

func TestNewPeriod_MonthWindow(t *testing.T) {
	p := NewPeriod(RouteOverview, SinglePeriodScope, presetsFor(PresetMonth, Preset6M), art(2026, time.July), art(2026, time.July), currency.ARS)

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
	p := NewPeriod(RouteEvolution, TrendScope, presetsFor(PresetMonth, Preset6M), art(2026, time.July), art(2026, time.July), currency.ARS)

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

	p := NewPeriod(RouteEvolution, TrendScope, presetsFor(PresetMonth, Preset3M), art(2026, time.January), art(2026, time.July), currency.ARS)

	if p.Label != "Nov 2025 – ene 2026" {
		t.Errorf("Label = %q, want %q", p.Label, "Nov 2025 – ene 2026")
	}
	if p.PrevQuery != "/app/evolution?c=ARS&m=2025-10&p=month&pt=3m" {
		t.Errorf("PrevQuery = %q", p.PrevQuery)
	}
	if p.NextQuery != "/app/evolution?c=ARS&m=2026-04&p=month&pt=3m" {
		t.Errorf("NextQuery = %q", p.NextQuery)
	}
}

func TestNewPeriod_NextClampedAtCurrentMonth(t *testing.T) {

	p := NewPeriod(RouteEvolution, TrendScope, presetsFor(PresetMonth, Preset3M), art(2026, time.May), art(2026, time.July), currency.ARS)
	if p.NextQuery != "" {
		t.Errorf("NextQuery = %q, want empty", p.NextQuery)
	}
}

func TestPeriod_LinkBuildersAreAbsolute(t *testing.T) {

	p := NewPeriod(RouteAccounts, TrendScope, presetsFor(PresetMonth, Preset6M), art(2026, time.July), art(2026, time.July), currency.ARS)

	if got := p.WithPreset(Preset3M); got != "/app/accounts?c=ARS&m=2026-07&p=month&pt=3m" {
		t.Errorf("WithPreset = %q", got)
	}
	if got := p.WithCurrency(currency.USD); got != "/app/accounts?c=USD&m=2026-07&p=month&pt=6m" {
		t.Errorf("WithCurrency = %q", got)
	}
	if got := p.Query(); got != "/app/accounts?c=ARS&m=2026-07&p=month&pt=6m" {
		t.Errorf("Query = %q", got)
	}
	if !p.IsPreset(Preset6M) || p.IsPreset(Preset3M) {
		t.Error("IsPreset debe marcar solo el preset activo")
	}
}

func TestWithDrill_HeaderLinksKeepTheLeaf(t *testing.T) {

	p := NewPeriod(RouteAccounts, SinglePeriodScope, presetsFor(PresetMonth, Preset6M), art(2026, time.June), art(2026, time.July), currency.ARS).
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

	if strings.Contains(p.Query(), "account=") {
		t.Errorf("Query() = %q, no debería llevar el drill", p.Query())
	}
}

func TestWithPreset_TouchesOnlyItsOwnScope(t *testing.T) {
	p := NewPeriod(RouteEvolution, TrendScope, presetsFor(PresetMonth, Preset6M),
		art(2026, time.July), art(2026, time.July), currency.ARS)

	if got := p.WithPreset(Preset3M); got != "/app/evolution?c=ARS&m=2026-07&p=month&pt=3m" {
		t.Errorf("WithPreset = %q", got)
	}
}

func TestWithPreset_DoesNotMutateThePeriod(t *testing.T) {

	base := presetsFor(PresetMonth, Preset6M)
	p := NewPeriod(RouteEvolution, TrendScope, base,
		art(2026, time.July), art(2026, time.July), currency.ARS)

	_ = p.WithPreset(Preset3M)

	if got := p.Presets[TrendScope.Param]; got != Preset6M {
		t.Errorf("WithPreset pisó el Period: Presets[pt] = %q, want 6m", got)
	}
	if got := base[TrendScope.Param]; got != Preset6M {
		t.Errorf("WithPreset pisó el map del caller: base[pt] = %q, want 6m", got)
	}
	if got := p.Query(); got != "/app/evolution?c=ARS&m=2026-07&p=month&pt=6m" {
		t.Errorf("Query después de WithPreset = %q", got)
	}
}

func TestDaysElapsed_ClosedMonthCountsEveryDayOfIt(t *testing.T) {
	p := NewPeriod(RouteCategories, SinglePeriodScope, presetsFor(PresetMonth, Preset6M),
		art(2026, time.July), art(2026, time.September), currency.ARS)

	if got := p.DaysElapsed(time.Date(2026, 9, 2, 10, 0, 0, 0, constants.ArgentinaZone)); got != 31 {
		t.Errorf("DaysElapsed = %d, want 31: julio ya cerró, se cuenta entero", got)
	}
}

func TestDaysElapsed_OpenMonthStopsToday(t *testing.T) {
	p := NewPeriod(RouteCategories, SinglePeriodScope, presetsFor(PresetMonth, Preset6M),
		art(2026, time.September), art(2026, time.September), currency.ARS)

	if got := p.DaysElapsed(time.Date(2026, 9, 2, 10, 0, 0, 0, constants.ArgentinaZone)); got != 2 {
		t.Errorf("DaysElapsed = %d, want 2: dividir por 30 informa un costo diario de 28 días que no pasaron", got)
	}
}

func TestDaysElapsed_MultiMonthAddsClosedMonthsPlusElapsed(t *testing.T) {
	p := NewPeriod(RouteCategories, SinglePeriodScope, presetsFor(Preset3M, Preset6M),
		art(2026, time.September), art(2026, time.September), currency.ARS)

	if got := p.DaysElapsed(time.Date(2026, 9, 2, 10, 0, 0, 0, constants.ArgentinaZone)); got != 64 {
		t.Errorf("DaysElapsed = %d, want 64: julio 31 + agosto 31 + 2 de septiembre", got)
	}
}

func TestDaysElapsed_FirstDayOfTheMonthCountsAsOne(t *testing.T) {
	p := NewPeriod(RouteCategories, SinglePeriodScope, presetsFor(PresetMonth, Preset6M),
		art(2026, time.September), art(2026, time.September), currency.ARS)

	if got := p.DaysElapsed(time.Date(2026, 9, 1, 0, 30, 0, 0, constants.ArgentinaZone)); got != 1 {
		t.Errorf("DaysElapsed = %d, want 1: el día en curso ya cuenta", got)
	}
}

func TestDaysElapsed_NeverReturnsZeroEvenIfTheWindowStartsInTheFuture(t *testing.T) {
	p := Period{
		From: art(2026, time.October),
		To:   art(2026, time.October).AddDate(0, 1, 0).Add(-time.Nanosecond),
	}

	if got := p.DaysElapsed(time.Date(2026, 9, 2, 10, 0, 0, 0, constants.ArgentinaZone)); got != 1 {
		t.Errorf("DaysElapsed = %d, want 1: un divisor 0 o negativo hace explotar decimal.Div", got)
	}
}

func TestDaysElapsed_UsesArgentineWallClockNotUTC(t *testing.T) {
	p := NewPeriod(RouteCategories, SinglePeriodScope, presetsFor(PresetMonth, Preset6M),
		art(2026, time.September), art(2026, time.September), currency.ARS)

	if got := p.DaysElapsed(time.Date(2026, 9, 3, 1, 30, 0, 0, time.UTC)); got != 2 {
		t.Errorf("DaysElapsed = %d, want 2: a la 01:30 UTC en Argentina todavía es el 2", got)
	}
}

func TestPerDayNote_SpansMonthsNamingBoth(t *testing.T) {
	p := NewPeriod(RouteCategories, SinglePeriodScope, presetsFor(Preset3M, Preset6M),
		art(2026, time.September), art(2026, time.September), currency.ARS)

	got := p.PerDayNote(time.Date(2026, 9, 2, 10, 0, 0, 0, constants.ArgentinaZone))
	want := "Por día = total ÷ 64 días corridos, del 1 jul al 2 sep."
	if got != want {
		t.Errorf("PerDayNote = %q, want %q", got, want)
	}
}

func TestPerDayNote_SameMonthPrintsTheMonthOnce(t *testing.T) {
	p := NewPeriod(RouteCategories, SinglePeriodScope, presetsFor(PresetMonth, Preset6M),
		art(2026, time.September), art(2026, time.September), currency.ARS)

	got := p.PerDayNote(time.Date(2026, 9, 2, 10, 0, 0, 0, constants.ArgentinaZone))
	want := "Por día = total ÷ 2 días corridos, del 1 al 2 sep."
	if got != want {
		t.Errorf("PerDayNote = %q, want %q", got, want)
	}
}

func TestPerDayNote_ClosedMonthEndsAtItsLastDay(t *testing.T) {
	p := NewPeriod(RouteCategories, SinglePeriodScope, presetsFor(PresetMonth, Preset6M),
		art(2026, time.July), art(2026, time.September), currency.ARS)

	got := p.PerDayNote(time.Date(2026, 9, 2, 10, 0, 0, 0, constants.ArgentinaZone))
	want := "Por día = total ÷ 31 días corridos, del 1 al 31 jul."
	if got != want {
		t.Errorf("PerDayNote = %q, want %q: un mes cerrado no termina hoy", got, want)
	}
}

func TestPerDayNote_NamesTheSameCountItDividesBy(t *testing.T) {
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, constants.ArgentinaZone)
	for _, preset := range []string{PresetMonth, Preset3M, Preset6M, PresetYear} {
		p := NewPeriod(RouteCategories, SinglePeriodScope, presetsFor(preset, Preset6M),
			art(2026, time.September), art(2026, time.September), currency.ARS)

		want := strconv.Itoa(p.DaysElapsed(now)) + " días corridos"
		if !strings.Contains(p.PerDayNote(now), want) {
			t.Errorf("preset %s: la nota dice %q y no contiene %q — la frase y la columna divergieron",
				preset, p.PerDayNote(now), want)
		}
	}
}
