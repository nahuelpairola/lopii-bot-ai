package notifier

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"lopiibot.com/internal/movement"
)

type fakePurger struct {
	purged  []movement.PurgedMovement
	err     error
	calls   int
	cutoffs []time.Time
}

func (f *fakePurger) PurgeSoftDeletedBefore(cutoff time.Time) ([]movement.PurgedMovement, error) {
	f.calls++
	f.cutoffs = append(f.cutoffs, cutoff)
	return f.purged, f.err
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestPurge_SoftDeletedMovementsOlderThan90DaysAreDeletedAndTheirIDsLogged(t *testing.T) {
	logs := captureLogs(t)
	p := &fakePurger{purged: []movement.PurgedMovement{{ID: 41, UserID: 7}, {ID: 42, UserID: 7}}}
	s := &Sweeper{purger: p}
	now := time.Date(2026, 10, 31, 3, 0, 0, 0, time.UTC)

	s.sweepSoftDeletedMovements(now)

	if p.calls != 1 {
		t.Fatalf("purge calls = %d, want 1", p.calls)
	}
	if want := time.Date(2026, 8, 2, 3, 0, 0, 0, time.UTC); !p.cutoffs[0].Equal(want) {
		t.Errorf("cutoff = %v, want %v", p.cutoffs[0], want)
	}
	out := logs.String()
	for _, want := range []string{"count=2", "ids=\"[41 42]\"", "user_ids=\"[7 7]\""} {
		if !strings.Contains(out, want) {
			t.Errorf("log %q does not contain %q", out, want)
		}
	}
	for _, leaked := range []string{"amount", "description"} {
		if strings.Contains(out, leaked) {
			t.Errorf("log %q must not carry %q", out, leaked)
		}
	}
}

func TestPurge_NothingToDeleteLogsNothing(t *testing.T) {
	logs := captureLogs(t)
	s := &Sweeper{purger: &fakePurger{}}

	s.sweepSoftDeletedMovements(time.Date(2026, 10, 31, 3, 0, 0, 0, time.UTC))

	if logs.Len() != 0 {
		t.Errorf("expected no log line, got %q", logs.String())
	}
}

func TestPurge_RunsAtMostOncePerDay(t *testing.T) {
	p := &fakePurger{}
	s := &Sweeper{purger: p}

	s.sweepSoftDeletedMovements(time.Date(2026, 10, 31, 3, 0, 0, 0, time.UTC))
	s.sweepSoftDeletedMovements(time.Date(2026, 10, 31, 23, 55, 0, 0, time.UTC))
	s.sweepSoftDeletedMovements(time.Date(2026, 11, 1, 0, 5, 0, 0, time.UTC))

	if p.calls != 2 {
		t.Errorf("purge calls = %d, want 2 (one per day)", p.calls)
	}
}

func TestPurge_FailureRetriesOnTheNextTickTheSameDay(t *testing.T) {
	p := &fakePurger{err: errors.New("db down")}
	s := &Sweeper{purger: p}

	s.sweepSoftDeletedMovements(time.Date(2026, 10, 31, 3, 0, 0, 0, time.UTC))
	p.err = nil
	s.sweepSoftDeletedMovements(time.Date(2026, 10, 31, 3, 5, 0, 0, time.UTC))
	s.sweepSoftDeletedMovements(time.Date(2026, 10, 31, 3, 10, 0, 0, time.UTC))

	if p.calls != 2 {
		t.Errorf("purge calls = %d, want 2 (the failed run retried, then gated)", p.calls)
	}
}
