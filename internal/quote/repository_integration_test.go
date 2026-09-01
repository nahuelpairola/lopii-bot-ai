//go:build integration

package quote

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/database"
)

func newTestRepo(t *testing.T) *repository {
	creds := database.Creds{
		Host:     "localhost",
		Name:     "lopiibot",
		Port:     5432,
		User:     "lopiibot",
		Password: "lopiibot",
	}
	conn, err := database.Initialize(creds, false)
	if err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}
	if err := conn.DB.Exec("DELETE FROM usd_quotes").Error; err != nil {
		t.Fatalf("failed to drain usd_quotes: %v", err)
	}
	return NewRepository(conn)
}

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestFindRateOnOrBefore_WalksBackOverAGap(t *testing.T) {
	r := newTestRepo(t)
	err := r.InsertQuotes([]Quote{
		{Date: day(2026, 7, 29), RateType: "bolsa", Bid: decimal.NewFromInt(1200), Ask: decimal.NewFromInt(1210)},
		{Date: day(2026, 7, 31), RateType: "oficial", Bid: decimal.NewFromInt(900), Ask: decimal.NewFromInt(910)},
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := r.FindRateOnOrBefore(day(2026, 7, 31), "bolsa")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil {
		t.Fatal("expected the 07-29 bolsa row, got nil")
	}
	if !got.Date.Equal(day(2026, 7, 29)) {
		t.Errorf("Date = %v, want 2026-07-29", got.Date)
	}
	if !got.Ask.Equal(decimal.NewFromInt(1210)) {
		t.Errorf("Ask = %s, want 1210", got.Ask)
	}
}

func TestFindRateOnOrBefore_NoRowReturnsNilNil(t *testing.T) {
	r := newTestRepo(t)
	got, err := r.FindRateOnOrBefore(day(2026, 7, 31), "bolsa")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestFindRateOnOrBefore_IgnoresLaterRows(t *testing.T) {
	r := newTestRepo(t)
	if err := r.InsertQuotes([]Quote{
		{Date: day(2026, 8, 5), RateType: "bolsa", Bid: decimal.NewFromInt(1300), Ask: decimal.NewFromInt(1310)},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, err := r.FindRateOnOrBefore(day(2026, 7, 31), "bolsa")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for a date before every row, got %+v", got)
	}
}
