package notifier

import (
	"context"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
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

type fakeMovements struct{ byUser map[uint64]int }

func (f *fakeMovements) FindRecentlyCreatedForUser(userID uint64, _ time.Time, limit int) ([]movement.Movement, error) {
	return make([]movement.Movement, f.byUser[userID]), nil
}

type fakeUsers struct{}

func (fakeUsers) FindByID(id uint64) (*user.User, error) {
	return &user.User{ID: id, TelegramID: "1000"}, nil
}

func newSweeper(r *fakeReminders, m *fakeMovements, sent *[]int64) *Sweeper {
	return &Sweeper{
		reminders: r,
		movements: m,
		users:     fakeUsers{},
		send: func(_ context.Context, chatID int64, _ string, _ *models.InlineKeyboardMarkup) error {
			*sent = append(*sent, chatID)
			return nil
		},
	}
}

func at(h, m int) time.Time { return time.Date(2026, 7, 9, h, m, 0, 0, time.UTC) }

func TestSweep_FiresPastMidpointWhenNoActivity(t *testing.T) {
	r := &fakeReminders{due: []reminder.Reminder{{UserID: 1, WindowStartMin: 1200, WindowEndMin: 1260}}} // 20:00-21:00, midpoint 20:30
	m := &fakeMovements{byUser: map[uint64]int{}}
	var sent []int64
	s := newSweeper(r, m, &sent)

	s.sweepReminders(context.Background(), at(20, 30)) // exactly midpoint
	if len(sent) != 1 || sent[0] != 1000 {
		t.Fatalf("expected one send to chat 1000, got %v", sent)
	}
	if _, ok := r.remindedT[1]; !ok {
		t.Error("expected SetLastRemindedOn to be called")
	}
}

func TestSweep_SkipsBeforeMidpoint(t *testing.T) {
	r := &fakeReminders{due: []reminder.Reminder{{UserID: 1, WindowStartMin: 1200, WindowEndMin: 1260}}}
	m := &fakeMovements{byUser: map[uint64]int{}}
	var sent []int64
	s := newSweeper(r, m, &sent)
	s.sweepReminders(context.Background(), at(20, 15)) // before 20:30
	if len(sent) != 0 {
		t.Fatalf("expected no send before midpoint, got %v", sent)
	}
}

func TestSweep_SkipsWhenLoggedToday(t *testing.T) {
	r := &fakeReminders{due: []reminder.Reminder{{UserID: 1, WindowStartMin: 1200, WindowEndMin: 1260}}}
	m := &fakeMovements{byUser: map[uint64]int{1: 3}} // logged 3 today
	var sent []int64
	s := newSweeper(r, m, &sent)
	s.sweepReminders(context.Background(), at(20, 45))
	if len(sent) != 0 {
		t.Fatalf("expected no send when already logged today, got %v", sent)
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
