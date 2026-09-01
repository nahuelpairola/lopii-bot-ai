package notifier

import (
	"context"
	"errors"
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

var errBoom = errors.New("la fuente se cayó")

type fakeQuoteAPI struct {
	allCalls   int
	dateCalls  []time.Time
	todayCalls int
	cpiCalls   int
	missing    map[string]bool
	err        error
}

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

func (f *fakeQuoteAPI) FetchCPI() ([]quote.CPI, error) {
	f.cpiCalls++
	if f.err != nil {
		return nil, f.err
	}
	return []quote.CPI{
		{Month: day(2011, time.January, 1)},
		{Month: day(2024, time.December, 1)},
		{Month: day(2025, time.January, 1)},
		{Month: day(2026, time.June, 1)},
	}, nil
}

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
	if len(s.inserted) != 2 {
		t.Fatalf("expected 2 seeded quotes (sólo 2026), got %d: %v", len(s.inserted), s.inserted)
	}
	for _, q := range s.inserted {
		if q.Date.Year() != 2026 {
			t.Errorf("seeded a quote from %d, el piso es 2026", q.Date.Year())
		}
	}
}

func TestSweepQuotes_UpToDateTableOnlyRefetchesTheSettleWindow(t *testing.T) {
	yesterday := day(2026, 8, 5)
	s, a := &fakeQuoteStore{latest: &yesterday}, &fakeQuoteAPI{}
	sw := newQuoteSweeper(s, a)

	sw.sweepQuotes(context.Background(), art(2026, 8, 6, 10, 0))

	if a.allCalls != 0 || a.todayCalls != 0 {
		t.Errorf("no debe sembrar ni pedir hoy: all=%d today=%d", a.allCalls, a.todayCalls)
	}
	if len(a.dateCalls) != 3 {
		t.Fatalf("expected 3 refetches, got %v", a.dateCalls)
	}
	for i, want := range []int{3, 4, 5} {
		if got := a.dateCalls[i].Day(); got != want {
			t.Errorf("call %d = day %d, want %d", i, got, want)
		}
	}
}

func TestSweepQuotes_RefetchesTheDayDolarAPIWrote(t *testing.T) {
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

func TestSweepCPI_InsertsFromTheSeedFloorOnward(t *testing.T) {
	s, a := &fakeQuoteStore{}, &fakeQuoteAPI{}
	sw := newQuoteSweeper(s, a)

	sw.sweepCPI(context.Background(), art(2026, 8, 6, 10, 0))

	if a.cpiCalls != 1 {
		t.Errorf("FetchCPI calls = %d, want 1", a.cpiCalls)
	}
	if len(s.cpi) != 2 {
		t.Fatalf("expected 2 months (2025 en adelante), got %d: %v", len(s.cpi), s.cpi)
	}
	for _, c := range s.cpi {
		if c.Month.Year() < 2025 {
			t.Errorf("inserted CPI de %d, el piso es 2025", c.Month.Year())
		}
	}
}

func TestSweepCPI_AfterBootWaitsForTheHour(t *testing.T) {
	s, a := &fakeQuoteStore{}, &fakeQuoteAPI{}
	sw := newQuoteSweeper(s, a)

	sw.sweepCPI(context.Background(), art(2026, 8, 6, 10, 0))
	sw.sweepCPI(context.Background(), art(2026, 8, 6, 19, 59))
	sw.sweepCPI(context.Background(), art(2026, 8, 7, 10, 0))

	if a.cpiCalls != 1 {
		t.Errorf("FetchCPI calls = %d, want 1 (sólo el arranque)", a.cpiCalls)
	}
}

func TestSweepCPI_RunsOncePerDayAtTheHour(t *testing.T) {
	s, a := &fakeQuoteStore{}, &fakeQuoteAPI{}
	sw := newQuoteSweeper(s, a)

	sw.sweepCPI(context.Background(), art(2026, 8, 6, 10, 0))
	sw.sweepCPI(context.Background(), art(2026, 8, 6, 20, 0))
	sw.sweepCPI(context.Background(), art(2026, 8, 6, 21, 0))
	sw.sweepCPI(context.Background(), art(2026, 8, 6, 23, 0))
	if a.cpiCalls != 2 {
		t.Fatalf("FetchCPI calls = %d, want 2 (arranque + 20:00)", a.cpiCalls)
	}

	sw.sweepCPI(context.Background(), art(2026, 8, 7, 20, 0))
	if a.cpiCalls != 3 {
		t.Errorf("FetchCPI calls = %d, want 3 (una por día)", a.cpiCalls)
	}
}

func TestSweepCPI_FailedRunRetriesAfterTheFloor(t *testing.T) {
	s, a := &fakeQuoteStore{}, &fakeQuoteAPI{err: errBoom}
	sw := newQuoteSweeper(s, a)

	sw.sweepCPI(context.Background(), art(2026, 8, 6, 10, 0))
	sw.sweepCPI(context.Background(), art(2026, 8, 6, 20, 0))
	sw.sweepCPI(context.Background(), art(2026, 8, 6, 20, 30))
	if a.cpiCalls != 2 {
		t.Fatalf("FetchCPI calls = %d, want 2: el piso frena el de 20:30", a.cpiCalls)
	}

	a.err = nil
	sw.sweepCPI(context.Background(), art(2026, 8, 6, 21, 0))
	if a.cpiCalls != 3 {
		t.Fatalf("FetchCPI calls = %d, want 3", a.cpiCalls)
	}
	sw.sweepCPI(context.Background(), art(2026, 8, 6, 22, 0))
	if a.cpiCalls != 3 {
		t.Errorf("FetchCPI calls = %d: una corrida exitosa cierra el día", a.cpiCalls)
	}
}

func TestSweepQuotes_RunsOncePerDayAtTheHour(t *testing.T) {
	last := day(2026, 8, 5)
	s, a := &fakeQuoteStore{latest: &last}, &fakeQuoteAPI{}
	sw := newQuoteSweeper(s, a)

	sw.sweepQuotes(context.Background(), art(2026, 8, 6, 10, 0))
	bootCalls := len(a.dateCalls)
	if bootCalls == 0 {
		t.Fatal("el arranque tiene que correr sea la hora que sea")
	}
	if a.todayCalls != 0 {
		t.Errorf("a las 10:00 no se pide el valor del día, calls = %d", a.todayCalls)
	}

	sw.sweepQuotes(context.Background(), art(2026, 8, 6, 15, 0))
	if len(a.dateCalls) != bootCalls {
		t.Errorf("no debe correr fuera de hora, calls = %v", a.dateCalls)
	}

	sw.sweepQuotes(context.Background(), art(2026, 8, 6, 20, 0))
	if len(a.dateCalls) == bootCalls {
		t.Error("a las 20:00 tiene que correr aunque haya corrido al arrancar")
	}
	if a.todayCalls != 1 {
		t.Errorf("todayCalls = %d, want 1", a.todayCalls)
	}

	after := len(a.dateCalls)
	sw.sweepQuotes(context.Background(), art(2026, 8, 6, 22, 0))
	if len(a.dateCalls) != after {
		t.Errorf("ya corrió hoy, no debe repetir: %v", a.dateCalls)
	}
}
