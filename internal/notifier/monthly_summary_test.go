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
	allUsers      []uint64
	monthlySentOn map[uint64]time.Time
}

func (s *mnStore) ListMonthlyDue(before time.Time) ([]uint64, error) {
	var out []uint64
	for _, id := range s.allUsers {
		if sent, ok := s.monthlySentOn[id]; !ok || sent.Before(before) {
			out = append(out, id)
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

func firstAt(hour, min int) time.Time {
	return time.Date(2026, 6, 1, hour, min, 0, 0, artLoc)
}

func TestSweepMonthlySummary_FiresOnTheFirst(t *testing.T) {
	store := &mnStore{allUsers: []uint64{1}}
	chat := &messenger.FakeChat{}

	sent := newMonthlySweeper(store, chat, "MONTHLY").sweepMonthlySummary(context.Background(), firstAt(9, 5))

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
	store := &mnStore{allUsers: []uint64{1}}
	chat := &messenger.FakeChat{}
	third := time.Date(2026, 6, 3, 9, 5, 0, 0, artLoc)

	newMonthlySweeper(store, chat, "MONTHLY").sweepMonthlySummary(context.Background(), third)

	if len(chat.Sent) != 0 {
		t.Fatalf("expected nothing on the 3rd, got %v", chat.Sent)
	}
}

func TestSweepMonthlySummary_NotBeforeNine(t *testing.T) {
	store := &mnStore{allUsers: []uint64{1}}
	chat := &messenger.FakeChat{}

	newMonthlySweeper(store, chat, "MONTHLY").sweepMonthlySummary(context.Background(), firstAt(8, 30))

	if len(chat.Sent) != 0 {
		t.Fatalf("expected nothing before 09:00, got %v", chat.Sent)
	}
}

func TestSweepMonthlySummary_EmptyMonthSendsNothingButStillMarks(t *testing.T) {
	store := &mnStore{allUsers: []uint64{1}}
	chat := &messenger.FakeChat{}

	sent := newMonthlySweeper(store, chat, "").sweepMonthlySummary(context.Background(), firstAt(9, 5))

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
	store := &mnStore{allUsers: []uint64{1}}
	s := newMonthlySweeper(store, &messenger.FakeChat{}, "MONTHLY")
	s.summaries = windowSpy{out: &got}

	s.sweepMonthlySummary(context.Background(), firstAt(9, 5))

	if got[0].Month() != time.May || got[0].Day() != 1 {
		t.Errorf("from = %v, want 2026-05-01", got[0])
	}
	if got[1].Month() != time.May || got[1].Day() != 31 {
		t.Errorf("to = %v, want 2026-05-31", got[1])
	}
	if got[2].Month() != time.April || got[2].Day() != 1 {
		t.Errorf("prevFrom = %v, want 2026-04-01", got[2])
	}
	if got[3].Month() != time.April || got[3].Day() != 30 {
		t.Errorf("prevTo = %v, want 2026-04-30", got[3])
	}
}

func TestSweepWeeklySummary_SkipsUsersTheMonthlyJustReached(t *testing.T) {
	monday1st := firstAt(9, 5)
	if monday1st.Weekday() != time.Monday {
		t.Fatalf("fixture is wrong: 2026-06-01 is a %v, pick a real Monday-the-1st", monday1st.Weekday())
	}
	store := &wkStore{due: []reminder.Reminder{{UserID: 1, WeeklySummaryEnabled: true}}}
	chat := &messenger.FakeChat{}

	newTestSweeper(store, chat).sweepWeeklySummary(context.Background(), monday1st, map[uint64]struct{}{1: {}})

	if len(chat.Sent) != 0 {
		t.Fatalf("the weekly must not double up on a Monday the 1st, got %v", chat.Sent)
	}
}
