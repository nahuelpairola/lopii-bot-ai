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

func (f *fakeQuoteAPI) FetchAll() ([]quote.Quote, error) {
	f.allCalls++
	return []quote.Quote{{Casa: "oficial"}}, nil
}
func (f *fakeQuoteAPI) FetchDate(d time.Time) ([]quote.Quote, error) {
	f.dateCalls = append(f.dateCalls, d)
	if f.missing[d.Format("2006-01-02")] {
		return nil, nil
	}
	return []quote.Quote{{Date: d, Casa: "oficial"}}, nil
}
func (f *fakeQuoteAPI) FetchToday(time.Time) ([]quote.Quote, error) {
	f.todayCalls++
	return []quote.Quote{{Casa: "oficial"}}, nil
}
func (f *fakeQuoteAPI) FetchCPI() ([]quote.CPI, error) {
	f.cpiCalls++
	return []quote.CPI{{}}, nil // el valor no importa acá, sólo que la lista llegue
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
	if len(s.inserted) == 0 {
		t.Error("expected the seeded quotes to be inserted")
	}
}

func TestSweepQuotes_UpToDateTableFetchesNothing(t *testing.T) {
	yesterday := day(2026, 8, 5)
	s, a := &fakeQuoteStore{latest: &yesterday}, &fakeQuoteAPI{}
	sw := newQuoteSweeper(s, a)

	// 10:00, antes del corte de las 20:00, así que tampoco pide el día de hoy.
	sw.sweepQuotes(context.Background(), art(2026, 8, 6, 10, 0))

	if a.allCalls != 0 || len(a.dateCalls) != 0 || a.todayCalls != 0 {
		t.Errorf("expected zero fetches, got all=%d dates=%v today=%d", a.allCalls, a.dateCalls, a.todayCalls)
	}
}

func TestSweepQuotes_ThreeDayGapFetchesThreeDates(t *testing.T) {
	last := day(2026, 8, 2)
	s, a := &fakeQuoteStore{latest: &last}, &fakeQuoteAPI{}
	sw := newQuoteSweeper(s, a)

	// Ayer es el 5; faltan 3, 4 y 5.
	sw.sweepQuotes(context.Background(), art(2026, 8, 6, 10, 0))

	if len(a.dateCalls) != 3 {
		t.Fatalf("expected 3 date fetches, got %v", a.dateCalls)
	}
	for i, want := range []int{3, 4, 5} {
		if got := a.dateCalls[i].Day(); got != want {
			t.Errorf("call %d = day %d, want %d", i, got, want)
		}
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
