package notifier

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/user"
)

type fakeReminders struct {
	due       []reminder.Reminder
	remindedT map[uint64]time.Time
}

func (f *fakeReminders) ListDue(time.Time) ([]reminder.Reminder, error) { return f.due, nil }
func (f *fakeReminders) SetLastRemindedOn(userID uint64, date time.Time) error {
	if f.remindedT == nil {
		f.remindedT = map[uint64]time.Time{}
	}
	f.remindedT[userID] = date
	return nil
}
func (f *fakeReminders) ListWeeklyDue(time.Time) ([]reminder.Reminder, error) { return nil, nil }
func (f *fakeReminders) SetLastSummaryOn(uint64, time.Time) error             { return nil }
func (f *fakeReminders) ListMonthlyDue(time.Time) ([]uint64, error)           { return nil, nil }
func (f *fakeReminders) SetLastMonthlySummaryOn(uint64, time.Time) error      { return nil }

type fakeMovements struct {
	byUser        map[uint64]int
	recurring     []string
	recurringErr  error
	recurringFrom time.Time
	recurringTo   time.Time
}

func (f *fakeMovements) FindRecentlyCreatedForUser(userID uint64, _ time.Time, limit int) ([]movement.Movement, error) {
	return make([]movement.Movement, f.byUser[userID]), nil
}

func (f *fakeMovements) TopRecurringDescriptions(_ uint64, from, to time.Time, _, _ int) ([]string, error) {
	f.recurringFrom, f.recurringTo = from, to
	return f.recurring, f.recurringErr
}

type fakeUsers struct{}

func (fakeUsers) FindByID(id uint64) (*user.User, error) {
	return &user.User{ID: id}, nil
}

type fakeChats struct{ chat *messenger.FakeChat }

func (f fakeChats) ChatFor(uint64) (messenger.Chat, error) { return f.chat, nil }

func newSweeper(r *fakeReminders, m *fakeMovements, chat *messenger.FakeChat) *Sweeper {
	return &Sweeper{
		reminders: r,
		movements: m,
		users:     fakeUsers{},
		chats:     fakeChats{chat: chat},
	}
}

func at(h, m int) time.Time { return time.Date(2026, 7, 9, h, m, 0, 0, time.UTC) }

func TestSweep_FiresPastMidpointWhenNoActivity(t *testing.T) {
	r := &fakeReminders{due: []reminder.Reminder{{UserID: 1, WindowStartMin: 1200, WindowEndMin: 1260}}}
	m := &fakeMovements{byUser: map[uint64]int{}}
	chat := &messenger.FakeChat{}
	s := newSweeper(r, m, chat)

	s.sweepReminders(context.Background(), at(20, 30))
	if len(chat.Sent) != 1 {
		t.Fatalf("expected one send, got %v", chat.Sent)
	}
	if chat.LastText() == "" {
		t.Error("expected a non-empty reminder text")
	}
	if _, ok := r.remindedT[1]; !ok {
		t.Error("expected SetLastRemindedOn to be called")
	}
}

func TestSweep_ReminderNamesTheUsersRecurringExpensesFromTheLast30Days(t *testing.T) {
	r := &fakeReminders{due: []reminder.Reminder{{UserID: 1, WindowStartMin: 1200, WindowEndMin: 1260}}}
	m := &fakeMovements{byUser: map[uint64]int{}, recurring: []string{"café", "panadería"}}
	chat := &messenger.FakeChat{}
	s := newSweeper(r, m, chat)

	s.sweepReminders(context.Background(), at(20, 30))
	if !strings.HasSuffix(chat.LastText(), "\n\nPor acá suele haber café o panadería.") {
		t.Errorf("reminder does not name the recurring expenses: %q", chat.LastText())
	}
	startOfDay := time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)
	if !m.recurringTo.Equal(startOfDay) || !m.recurringFrom.Equal(startOfDay.AddDate(0, 0, -30)) {
		t.Errorf("window = [%v, %v], want the 30 days up to %v", m.recurringFrom, m.recurringTo, startOfDay)
	}
}

func TestSweep_RecurringLookupFailureStillSendsThePlainReminder(t *testing.T) {
	r := &fakeReminders{due: []reminder.Reminder{{UserID: 1, WindowStartMin: 1200, WindowEndMin: 1260}}}
	m := &fakeMovements{byUser: map[uint64]int{}, recurring: []string{"café", "panadería"}, recurringErr: errors.New("db down")}
	chat := &messenger.FakeChat{}
	s := newSweeper(r, m, chat)

	s.sweepReminders(context.Background(), at(20, 30))
	if len(chat.Sent) != 1 {
		t.Fatalf("expected one send despite the lookup failure, got %v", chat.Sent)
	}
	if strings.Contains(chat.LastText(), "Por acá suele haber") {
		t.Errorf("failed lookup must not enrich: %q", chat.LastText())
	}
	if _, ok := r.remindedT[1]; !ok {
		t.Error("expected SetLastRemindedOn to be called")
	}
}

func TestSweep_SkipsBeforeMidpoint(t *testing.T) {
	r := &fakeReminders{due: []reminder.Reminder{{UserID: 1, WindowStartMin: 1200, WindowEndMin: 1260}}}
	m := &fakeMovements{byUser: map[uint64]int{}}
	chat := &messenger.FakeChat{}
	s := newSweeper(r, m, chat)
	s.sweepReminders(context.Background(), at(20, 15))
	if len(chat.Sent) != 0 {
		t.Fatalf("expected no send before midpoint, got %v", chat.Sent)
	}
}

func TestSweep_SkipsWhenLoggedToday(t *testing.T) {
	r := &fakeReminders{due: []reminder.Reminder{{UserID: 1, WindowStartMin: 1200, WindowEndMin: 1260}}}
	m := &fakeMovements{byUser: map[uint64]int{1: 3}}
	chat := &messenger.FakeChat{}
	s := newSweeper(r, m, chat)
	s.sweepReminders(context.Background(), at(20, 45))
	if len(chat.Sent) != 0 {
		t.Fatalf("expected no send when already logged today, got %v", chat.Sent)
	}
}

type fakeRetention struct {
	cutoff time.Time
	called bool
}

func (f *fakeRetention) DeleteOlderThan(c time.Time) error { f.cutoff, f.called = c, true; return nil }

func TestSweepRetentionDeletesOldRows(t *testing.T) {
	ret := &fakeRetention{}
	s := &Sweeper{retention: ret}
	now := time.Date(2026, 7, 13, 4, 0, 0, 0, time.UTC)
	s.sweepRetention(now)
	if !ret.called {
		t.Fatal("retention not invoked")
	}
	want := now.AddDate(0, 0, -retentionDays)
	if !ret.cutoff.Equal(want) {
		t.Fatalf("cutoff %v, want %v", ret.cutoff, want)
	}
}
