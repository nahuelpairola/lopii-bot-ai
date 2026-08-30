package notifier

import (
	"context"
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
func (f *fakeReminders) ListWeeklyDue(time.Time) ([]reminder.Reminder, error)  { return nil, nil }
func (f *fakeReminders) SetLastSummaryOn(uint64, time.Time) error              { return nil }
func (f *fakeReminders) ListMonthlyDue(time.Time) ([]reminder.Reminder, error) { return nil, nil }
func (f *fakeReminders) SetLastMonthlySummaryOn(uint64, time.Time) error       { return nil }

type fakeMovements struct{ byUser map[uint64]int }

func (f *fakeMovements) FindRecentlyCreatedForUser(userID uint64, _ time.Time, limit int) ([]movement.Movement, error) {
	return make([]movement.Movement, f.byUser[userID]), nil
}

type fakeUsers struct{}

func (fakeUsers) FindByID(id uint64) (*user.User, error) {
	return &user.User{ID: id}, nil
}

// fakeChats es el chatResolver de los tests: siempre devuelve el mismo
// *messenger.FakeChat, así los tests afirman sobre chat.Sent en vez de
// interceptar el (chatID, texto) que viajaba por el viejo `send`.
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
	r := &fakeReminders{due: []reminder.Reminder{{UserID: 1, WindowStartMin: 1200, WindowEndMin: 1260}}} // 20:00-21:00, midpoint 20:30
	m := &fakeMovements{byUser: map[uint64]int{}}
	chat := &messenger.FakeChat{}
	s := newSweeper(r, m, chat)

	s.sweepReminders(context.Background(), at(20, 30)) // exactly midpoint
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

func TestSweep_SkipsBeforeMidpoint(t *testing.T) {
	r := &fakeReminders{due: []reminder.Reminder{{UserID: 1, WindowStartMin: 1200, WindowEndMin: 1260}}}
	m := &fakeMovements{byUser: map[uint64]int{}}
	chat := &messenger.FakeChat{}
	s := newSweeper(r, m, chat)
	s.sweepReminders(context.Background(), at(20, 15)) // before 20:30
	if len(chat.Sent) != 0 {
		t.Fatalf("expected no send before midpoint, got %v", chat.Sent)
	}
}

func TestSweep_SkipsWhenLoggedToday(t *testing.T) {
	r := &fakeReminders{due: []reminder.Reminder{{UserID: 1, WindowStartMin: 1200, WindowEndMin: 1260}}}
	m := &fakeMovements{byUser: map[uint64]int{1: 3}} // logged 3 today
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
