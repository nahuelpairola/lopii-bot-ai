package notifier

import (
	"context"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
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
func (s *wkStore) SetLastSummaryOn(userID uint64, date time.Time) error {
	if s.sentOn == nil {
		s.sentOn = map[uint64]time.Time{}
	}
	s.sentOn[userID] = date
	return nil
}

type wkUsers struct{}

func (wkUsers) FindByID(id uint64) (*user.User, error) {
	return &user.User{ID: id, TelegramID: "555"}, nil
}

type wkSummary struct{ text string }

func (w wkSummary) Build(uint64, time.Time, time.Time, time.Time, time.Time) (string, error) {
	return w.text, nil
}

func mondayAt(hour, min int) time.Time {
	// 2026-07-13 is a Monday (ART).
	return time.Date(2026, 7, 13, hour, min, 0, 0, artLoc)
}

func newTestSweeper(store *wkStore, sent *[]string) *Sweeper {
	return &Sweeper{
		reminders: store,
		users:     wkUsers{},
		summaries: wkSummary{text: "REPORT"},
		send: func(_ context.Context, _ int64, text string, _ *models.InlineKeyboardMarkup) error {
			*sent = append(*sent, text)
			return nil
		},
	}
}

func TestSweepWeeklySummary_FiresMondayMorning(t *testing.T) {
	store := &wkStore{due: []reminder.Reminder{{UserID: 1, WeeklySummaryEnabled: true}}}
	var sent []string
	s := newTestSweeper(store, &sent)

	s.sweepWeeklySummary(context.Background(), mondayAt(9, 5))

	if len(sent) != 1 || sent[0] != "REPORT" {
		t.Fatalf("expected one REPORT sent, got %v", sent)
	}
	if _, ok := store.sentOn[1]; !ok {
		t.Fatalf("expected last_summary_on set for user 1")
	}
}

func TestSweepWeeklySummary_NotBeforeNine(t *testing.T) {
	store := &wkStore{due: []reminder.Reminder{{UserID: 1, WeeklySummaryEnabled: true}}}
	var sent []string
	newTestSweeper(store, &sent).sweepWeeklySummary(context.Background(), mondayAt(8, 30))
	if len(sent) != 0 {
		t.Fatalf("expected nothing before 09:00, got %v", sent)
	}
}

func TestSweepWeeklySummary_SkipsNonMonday(t *testing.T) {
	store := &wkStore{due: []reminder.Reminder{{UserID: 1, WeeklySummaryEnabled: true}}}
	var sent []string
	tuesday := time.Date(2026, 7, 14, 9, 5, 0, 0, artLoc)
	newTestSweeper(store, &sent).sweepWeeklySummary(context.Background(), tuesday)
	if len(sent) != 0 {
		t.Fatalf("expected nothing on non-Monday, got %v", sent)
	}
}
