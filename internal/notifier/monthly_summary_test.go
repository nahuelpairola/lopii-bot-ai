package notifier

import (
	"context"
	"testing"
	"time"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/reminder"
)

type mnStore struct {
	wkStore
	monthlySentOn map[uint64]time.Time
}

func (s *mnStore) ListMonthlyDue(before time.Time) ([]reminder.Reminder, error) {
	var out []reminder.Reminder
	for _, r := range s.due {
		if r.LastMonthlySummaryOn == nil || r.LastMonthlySummaryOn.Before(before) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *mnStore) SetLastMonthlySummaryOn(userID uint64, date time.Time) error {
	if s.monthlySentOn == nil {
		s.monthlySentOn = map[uint64]time.Time{}
	}
	s.monthlySentOn[userID] = date
	return nil
}

type mnSummary struct{ text string }

func (mnSummary) Build(uint64, time.Time, time.Time, time.Time, time.Time) (string, error) {
	return "WEEKLY", nil
}

func (m mnSummary) BuildMonthly(uint64, time.Time, time.Time, time.Time, time.Time) (conversation.Prompt, error) {
	return conversation.Prompt{Text: m.text}, nil
}

func newMonthlySweeper(store *mnStore, chat *messenger.FakeChat, text string) *Sweeper {
	return &Sweeper{
		reminders: store,
		users:     wkUsers{},
		summaries: mnSummary{text: text},
		chats:     fakeChats{chat: chat},
	}
}

func thirdAt(hour, min int) time.Time {
	return time.Date(2026, 8, 3, hour, min, 0, 0, artLoc)
}

func TestSweepMonthlySummary_FiresOnTheThird(t *testing.T) {
	store := &mnStore{wkStore: wkStore{due: []reminder.Reminder{{UserID: 1, WeeklySummaryEnabled: true}}}}
	chat := &messenger.FakeChat{}

	sent := newMonthlySweeper(store, chat, "MONTHLY").sweepMonthlySummary(context.Background(), thirdAt(9, 5))

	if len(chat.Sent) != 1 || chat.LastText() != "MONTHLY" {
		t.Fatalf("expected one MONTHLY, got %v", chat.Sent)
	}
	if _, ok := store.monthlySentOn[1]; !ok {
		t.Fatal("expected last_monthly_summary_on set")
	}
	if _, ok := sent[1]; !ok {
		t.Fatal("expected user 1 in the suppression set")
	}
}

func TestSweepMonthlySummary_SkipsOtherDays(t *testing.T) {
	store := &mnStore{wkStore: wkStore{due: []reminder.Reminder{{UserID: 1, WeeklySummaryEnabled: true}}}}
	chat := &messenger.FakeChat{}
	fourth := time.Date(2026, 8, 4, 9, 5, 0, 0, artLoc)

	newMonthlySweeper(store, chat, "MONTHLY").sweepMonthlySummary(context.Background(), fourth)

	if len(chat.Sent) != 0 {
		t.Fatalf("expected nothing on the 4th, got %v", chat.Sent)
	}
}

func TestSweepMonthlySummary_NotBeforeNine(t *testing.T) {
	store := &mnStore{wkStore: wkStore{due: []reminder.Reminder{{UserID: 1, WeeklySummaryEnabled: true}}}}
	chat := &messenger.FakeChat{}

	newMonthlySweeper(store, chat, "MONTHLY").sweepMonthlySummary(context.Background(), thirdAt(8, 30))

	if len(chat.Sent) != 0 {
		t.Fatalf("expected nothing before 09:00, got %v", chat.Sent)
	}
}

func TestSweepMonthlySummary_EmptyMonthSendsNothingButStillMarks(t *testing.T) {
	store := &mnStore{wkStore: wkStore{due: []reminder.Reminder{{UserID: 1, WeeklySummaryEnabled: true}}}}
	chat := &messenger.FakeChat{}

	sent := newMonthlySweeper(store, chat, "").sweepMonthlySummary(context.Background(), thirdAt(9, 5))

	if len(chat.Sent) != 0 {
		t.Fatalf("an empty month must send nothing, got %v", chat.Sent)
	}
	if _, ok := store.monthlySentOn[1]; !ok {
		t.Fatal("expected the day still marked so the tick does not re-evaluate all day")
	}
	if _, ok := sent[1]; ok {
		t.Fatal("a user who got nothing must NOT have their weekly suppressed")
	}
}

type windowSpy struct{ out *[4]time.Time }

func (windowSpy) Build(uint64, time.Time, time.Time, time.Time, time.Time) (string, error) {
	return "", nil
}

func (w windowSpy) BuildMonthly(_ uint64, from, to, prevFrom, prevTo time.Time) (conversation.Prompt, error) {
	*w.out = [4]time.Time{from, to, prevFrom, prevTo}
	return conversation.Prompt{Text: "X"}, nil
}

func TestSweepMonthlySummary_WindowIsThePreviousCalendarMonth(t *testing.T) {
	var got [4]time.Time
	store := &mnStore{wkStore: wkStore{due: []reminder.Reminder{{UserID: 1, WeeklySummaryEnabled: true}}}}
	s := newMonthlySweeper(store, &messenger.FakeChat{}, "MONTHLY")
	s.summaries = windowSpy{out: &got}

	s.sweepMonthlySummary(context.Background(), thirdAt(9, 5))

	if got[0].Month() != time.July || got[0].Day() != 1 {
		t.Errorf("from = %v, want 2026-07-01", got[0])
	}
	if got[1].Month() != time.July || got[1].Day() != 31 {
		t.Errorf("to = %v, want 2026-07-31", got[1])
	}
	if got[2].Month() != time.June || got[2].Day() != 1 {
		t.Errorf("prevFrom = %v, want 2026-06-01", got[2])
	}
	if got[3].Month() != time.June || got[3].Day() != 30 {
		t.Errorf("prevTo = %v, want 2026-06-30", got[3])
	}
}

func TestSweepWeeklySummary_SkipsUsersTheMonthlyJustReached(t *testing.T) {
	monday3rd := time.Date(2026, 8, 3, 9, 5, 0, 0, artLoc)
	if monday3rd.Weekday() != time.Monday {
		t.Fatalf("fixture is wrong: 2026-08-03 is a %v, pick a real Monday-the-3rd", monday3rd.Weekday())
	}
	store := &wkStore{due: []reminder.Reminder{{UserID: 1, WeeklySummaryEnabled: true}}}
	chat := &messenger.FakeChat{}

	newTestSweeper(store, chat).sweepWeeklySummary(context.Background(), monday3rd, map[uint64]struct{}{1: {}})

	if len(chat.Sent) != 0 {
		t.Fatalf("the weekly must not double up on a Monday the 3rd, got %v", chat.Sent)
	}
}
