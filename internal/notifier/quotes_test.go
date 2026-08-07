package notifier

import (
	"context"
	"testing"
	"time"

	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/quote"
)

type fakeQuoteStore struct {
	latest   *time.Time
	inserted []quote.Quote
	cpi      []quote.CPI
}

func (f *fakeQuoteStore) LatestQuoteDate() (*time.Time, error) { return f.latest, nil }
func (f *fakeQuoteStore) InsertQuotes(qs []quote.Quote) error {
	f.inserted = append(f.inserted, qs...)
	return nil
}
func (f *fakeQuoteStore) InsertCPI(cs []quote.CPI) error {
	f.cpi = append(f.cpi, cs...)
	return nil
}

type fakeQuoteAPI struct {
	allCalls   int
	dateCalls  []time.Time
	todayCalls int
	cpiCalls   int
	missing    map[string]bool // fechas que devuelven 404
}

// FetchAll imita a la fuente real: historia larga, desde mucho antes del piso
// del sembrado. Sólo las dos últimas filas deberían sobrevivir al filtro.
func (f *fakeQuoteAPI) FetchAll() ([]quote.Quote, error) {
	f.allCalls++
	return []quote.Quote{
		{Date: day(2011, time.January, 3), RateType: "oficial"},
		{Date: day(2025, time.December, 31), RateType: "oficial"},
		{Date: day(2026, time.January, 1), RateType: "oficial"},
		{Date: day(2026, time.August, 5), RateType: "oficial"},
	}, nil
}
func (f *fakeQuoteAPI) FetchDate(d time.Time) ([]quote.Quote, error) {
	f.dateCalls = append(f.dateCalls, d)
	if f.missing[d.Format("2006-01-02")] {
		return nil, nil
	}
	return []quote.Quote{{Date: d, RateType: "oficial"}}, nil
}
func (f *fakeQuoteAPI) FetchToday(time.Time) ([]quote.Quote, error) {
	f.todayCalls++
	return []quote.Quote{{RateType: "oficial"}}, nil
}

// FetchCPI, igual que la real, arranca en 2011. El piso deja pasar 2025 y 2026.
func (f *fakeQuoteAPI) FetchCPI() ([]quote.CPI, error) {
	f.cpiCalls++
	return []quote.CPI{
		{Month: day(2011, time.January, 1)},
		{Month: day(2024, time.December, 1)},
		{Month: day(2025, time.January, 1)},
		{Month: day(2026, time.June, 1)},
	}, nil
}

// art construye un instante en la zona horaria en la que corre el sweeper.
func art(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, constants.ArgentinaZone)
}

func day(y int, mo time.Month, d int) time.Time { return art(y, mo, d, 0, 0) }

func newQuoteSweeper(s *fakeQuoteStore, a *fakeQuoteAPI) *Sweeper {
	return &Sweeper{quotes: s, quoteAPI: a}
}

func TestSweepQuotes_EmptyTableSeedsWithOneFullFetch(t *testing.T) {
	s, a := &fakeQuoteStore{latest: nil}, &fakeQuoteAPI{}
	sw := newQuoteSweeper(s, a)

	sw.sweepQuotes(context.Background(), art(2026, 8, 6, 10, 0))

	if a.allCalls != 1 {
		t.Errorf("FetchAll calls = %d, want 1", a.allCalls)
	}
	if len(a.dateCalls) != 0 {
		t.Errorf("expected no per-date calls on seed, got %v", a.dateCalls)
	}
	// Sólo el año corriente: 2011 y 2025 quedan afuera, 2026 entra.
	if len(s.inserted) != 2 {
		t.Fatalf("expected 2 seeded quotes (sólo 2026), got %d: %v", len(s.inserted), s.inserted)
	}
	for _, q := range s.inserted {
		if q.Date.Year() != 2026 {
			t.Errorf("seeded a quote from %d, el piso es 2026", q.Date.Year())
		}
	}
}

// Una tabla al día no vuelve a sembrar ni pide el día de hoy antes de las
// 20:00, pero SÍ vuelve a pedir la ventana de asentamiento: son los días que
// pudo haber escrito dolarapi y que el histórico tiene que corregir.
func TestSweepQuotes_UpToDateTableOnlyRefetchesTheSettleWindow(t *testing.T) {
	yesterday := day(2026, 8, 5)
	s, a := &fakeQuoteStore{latest: &yesterday}, &fakeQuoteAPI{}
	sw := newQuoteSweeper(s, a)

	sw.sweepQuotes(context.Background(), art(2026, 8, 6, 10, 0))

	if a.allCalls != 0 || a.todayCalls != 0 {
		t.Errorf("no debe sembrar ni pedir hoy: all=%d today=%d", a.allCalls, a.todayCalls)
	}
	// Ventana de 3 días desde el 6: 3, 4 y 5. Nada del 2 para atrás.
	if len(a.dateCalls) != 3 {
		t.Fatalf("expected 3 refetches, got %v", a.dateCalls)
	}
	for i, want := range []int{3, 4, 5} {
		if got := a.dateCalls[i].Day(); got != want {
			t.Errorf("call %d = day %d, want %d", i, got, want)
		}
	}
}

// La razón de ser de la ventana: dolarapi escribió el día de hoy, así que al
// día siguiente latest ES esa fecha. Sin la ventana el loop arrancaría en
// latest+1 y ese valor provisorio no se volvería a pedir nunca.
func TestSweepQuotes_RefetchesTheDayDolarAPIWrote(t *testing.T) {
	// Ayer a las 20:00 dolarapi escribió el 2026-08-05; hoy es el 6.
	provisional := day(2026, 8, 5)
	s, a := &fakeQuoteStore{latest: &provisional}, &fakeQuoteAPI{}
	sw := newQuoteSweeper(s, a)

	sw.sweepQuotes(context.Background(), art(2026, 8, 6, 10, 0))

	var refetched bool
	for _, d := range a.dateCalls {
		if d.Equal(provisional) {
			refetched = true
		}
	}
	if !refetched {
		t.Errorf("el día que escribió dolarapi tiene que volver a pedirse al histórico, calls = %v", a.dateCalls)
	}
}

// El hueco arranca ANTES que la ventana de asentamiento, así que este test
// prueba el camino de hueco y no el de ventana: con last = 29/7 el loop tiene
// que ir del 30/7 al 5/8, siete fechas, no las tres de la ventana.
func TestSweepQuotes_LongGapFetchesEveryMissingDate(t *testing.T) {
	last := day(2026, 7, 29)
	s, a := &fakeQuoteStore{latest: &last}, &fakeQuoteAPI{}
	sw := newQuoteSweeper(s, a)

	sw.sweepQuotes(context.Background(), art(2026, 8, 6, 10, 0))

	if len(a.dateCalls) != 7 {
		t.Fatalf("expected 7 date fetches, got %v", a.dateCalls)
	}
	if got := a.dateCalls[0].Format("2006-01-02"); got != "2026-07-30" {
		t.Errorf("primera fecha = %s, want 2026-07-30 (el hueco, no la ventana)", got)
	}
	if got := a.dateCalls[6].Format("2006-01-02"); got != "2026-08-05" {
		t.Errorf("última fecha = %s, want 2026-08-05 (ayer)", got)
	}
}

func TestSweepQuotes_MissingDateDoesNotAbortTheRest(t *testing.T) {
	last := day(2026, 8, 2)
	s := &fakeQuoteStore{latest: &last}
	a := &fakeQuoteAPI{missing: map[string]bool{"2026-08-03": true}}
	sw := newQuoteSweeper(s, a)

	sw.sweepQuotes(context.Background(), art(2026, 8, 6, 10, 0))

	if len(a.dateCalls) != 3 {
		t.Errorf("a 404 must not stop the loop, calls = %v", a.dateCalls)
	}
	if len(s.inserted) != 2 {
		t.Errorf("expected 2 inserted quotes (el 3 no cotizó), got %d", len(s.inserted))
	}
}

func TestSweepQuotes_TodayFetchedOnlyFrom20hART(t *testing.T) {
	yesterday := day(2026, 8, 5)

	s, a := &fakeQuoteStore{latest: &yesterday}, &fakeQuoteAPI{}
	newQuoteSweeper(s, a).sweepQuotes(context.Background(), art(2026, 8, 6, 19, 59))
	if a.todayCalls != 0 {
		t.Errorf("19:59 must not fetch today, calls = %d", a.todayCalls)
	}

	s2, a2 := &fakeQuoteStore{latest: &yesterday}, &fakeQuoteAPI{}
	newQuoteSweeper(s2, a2).sweepQuotes(context.Background(), art(2026, 8, 6, 20, 0))
	if a2.todayCalls != 1 {
		t.Errorf("20:00 must fetch today, calls = %d", a2.todayCalls)
	}
}

func TestSweepQuotes_SecondCallWithinTheHourFetchesNothing(t *testing.T) {
	last := day(2026, 8, 2)
	s, a := &fakeQuoteStore{latest: &last}, &fakeQuoteAPI{}
	sw := newQuoteSweeper(s, a)

	sw.sweepQuotes(context.Background(), art(2026, 8, 6, 10, 0))
	before := len(a.dateCalls)
	sw.sweepQuotes(context.Background(), art(2026, 8, 6, 10, 5))

	if len(a.dateCalls) != before {
		t.Errorf("throttle broken: calls went %d -> %d", before, len(a.dateCalls))
	}
}

func TestSweepCPI_InsertsFromTheSeedFloorOnward(t *testing.T) {
	s, a := &fakeQuoteStore{}, &fakeQuoteAPI{}
	sw := newQuoteSweeper(s, a)

	sw.sweepCPI(context.Background(), art(2026, 8, 6, 10, 0))

	if a.cpiCalls != 1 {
		t.Errorf("FetchCPI calls = %d, want 1", a.cpiCalls)
	}
	// Desde el año pasado: 2011 y 2024 quedan afuera, 2025 y 2026 entran.
	if len(s.cpi) != 2 {
		t.Fatalf("expected 2 months (2025 en adelante), got %d: %v", len(s.cpi), s.cpi)
	}
	for _, c := range s.cpi {
		if c.Month.Year() < 2025 {
			t.Errorf("inserted CPI de %d, el piso es 2025", c.Month.Year())
		}
	}
}

func TestSweepCPI_SecondCallWithin24hDoesNothing(t *testing.T) {
	s, a := &fakeQuoteStore{}, &fakeQuoteAPI{}
	sw := newQuoteSweeper(s, a)

	sw.sweepCPI(context.Background(), art(2026, 8, 6, 10, 0))
	sw.sweepCPI(context.Background(), art(2026, 8, 7, 9, 0)) // 23 h después

	if a.cpiCalls != 1 {
		t.Errorf("throttle broken: FetchCPI calls = %d, want 1", a.cpiCalls)
	}
}

func TestSweepCPI_FiresAgainAfter24h(t *testing.T) {
	s, a := &fakeQuoteStore{}, &fakeQuoteAPI{}
	sw := newQuoteSweeper(s, a)

	sw.sweepCPI(context.Background(), art(2026, 8, 6, 10, 0))
	sw.sweepCPI(context.Background(), art(2026, 8, 7, 10, 0))

	if a.cpiCalls != 2 {
		t.Errorf("FetchCPI calls = %d, want 2", a.cpiCalls)
	}
}
