package notifier

import (
	"context"
	"testing"
	"time"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/user"
)

type wkStore struct {
	due    []reminder.Reminder
	sentOn map[uint64]time.Time
}

func (s *wkStore) ListDue(time.Time) ([]reminder.Reminder, error) { return nil, nil }
func (s *wkStore) SetLastRemindedOn(uint64, time.Time) error      { return nil }
func (s *wkStore) ListWeeklyDue(before time.Time) ([]reminder.Reminder, error) {
	var out []reminder.Reminder
	for _, r := range s.due {
		if r.LastSummaryOn == nil || r.LastSummaryOn.Before(before) {
			out = append(out, r)
		}
	}
	return out, nil
}
func (s *wkStore) ListMonthlyDue(time.Time) ([]uint64, error)      { return nil, nil }
func (s *wkStore) SetLastMonthlySummaryOn(uint64, time.Time) error { return nil }
func (s *wkStore) SetLastSummaryOn(userID uint64, date time.Time) error {
	if s.sentOn == nil {
		s.sentOn = map[uint64]time.Time{}
	}
	s.sentOn[userID] = date
	return nil
}

type wkUsers struct{}

func (wkUsers) FindByID(id uint64) (*user.User, error) {
	return &user.User{ID: id}, nil
}

type wkSummary struct{ text string }

func (wkSummary) BuildMonthly(uint64, time.Time, time.Time, time.Time, time.Time) (conversation.Prompt, error) {
	return conversation.Prompt{}, nil
}

func (w wkSummary) Build(uint64, time.Time, time.Time, time.Time, time.Time) (string, error) {
	return w.text, nil
}

func mondayAt(hour, min int) time.Time {
	// 2026-07-13 is a Monday (ART).
	return time.Date(2026, 7, 13, hour, min, 0, 0, artLoc)
}

func newTestSweeper(store *wkStore, chat *messenger.FakeChat) *Sweeper {
	return &Sweeper{
		reminders: store,
		users:     wkUsers{},
		summaries: wkSummary{text: "REPORT"},
		chats:     fakeChats{chat: chat},
	}
}

func TestSweepWeeklySummary_FiresMondayMorning(t *testing.T) {
	store := &wkStore{due: []reminder.Reminder{{UserID: 1, WeeklySummaryEnabled: true}}}
	chat := &messenger.FakeChat{}
	s := newTestSweeper(store, chat)

	s.sweepWeeklySummary(context.Background(), mondayAt(9, 5), nil)

	if len(chat.Sent) != 1 || chat.LastText() != "REPORT" {
		t.Fatalf("expected one REPORT sent, got %v", chat.Sent)
	}
	if _, ok := store.sentOn[1]; !ok {
		t.Fatalf("expected last_summary_on set for user 1")
	}
}

func TestSweepWeeklySummary_NotBeforeNine(t *testing.T) {
	store := &wkStore{due: []reminder.Reminder{{UserID: 1, WeeklySummaryEnabled: true}}}
	chat := &messenger.FakeChat{}
	newTestSweeper(store, chat).sweepWeeklySummary(context.Background(), mondayAt(8, 30), nil)
	if len(chat.Sent) != 0 {
		t.Fatalf("expected nothing before 09:00, got %v", chat.Sent)
	}
}

func TestSweepWeeklySummary_SkipsNonMonday(t *testing.T) {
	store := &wkStore{due: []reminder.Reminder{{UserID: 1, WeeklySummaryEnabled: true}}}
	chat := &messenger.FakeChat{}
	tuesday := time.Date(2026, 7, 14, 9, 5, 0, 0, artLoc)
	newTestSweeper(store, chat).sweepWeeklySummary(context.Background(), tuesday, nil)
	if len(chat.Sent) != 0 {
		t.Fatalf("expected nothing on non-Monday, got %v", chat.Sent)
	}
}
