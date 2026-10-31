package nudges

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
	"lopiibot.com/internal/user"
)

type testServices struct {
	counts       int64
	dayCounts    []movement.DayCount
	sumRows      func(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error)
	accountsBy   []account.Account
	remindersBy  map[uint64]*reminder.Reminder
	usersBy      map[uint64]*user.User
	engineActive bool
	available    *bool
	sentKeys     map[string]bool
	lastSentAt   map[uint64]time.Time
	marked       []string
	tapped       []string
	asked        string
	queryAnswer  string
}

func (t *testServices) tips() *Tips {
	var history sentHistory = t
	if t.available != nil && !*t.available {
		history = nil
	}
	return New(t, testReminders{t}, t, testAccounts{t}, t, t, history, t.HandleQuery)
}

type testReminders struct{ t *testServices }

func (r testReminders) FindByUserID(userID uint64) (*reminder.Reminder, error) {
	if rem, ok := r.t.remindersBy[userID]; ok {
		return rem, nil
	}
	return nil, nil
}

type testAccounts struct{ t *testServices }

func (a testAccounts) FindByUserID(uint64) ([]account.Account, error) {
	return a.t.accountsBy, nil
}

func sentTexts(c *messenger.FakeChat) []string {
	texts := make([]string, 0, len(c.Sent))
	for _, p := range c.Sent {
		texts = append(texts, p.Text)
	}
	return texts
}

func sentMarkups(c *messenger.FakeChat) []string {
	markups := make([]string, 0, len(c.Sent))
	for _, p := range c.Sent {
		rows := make([]string, 0, len(p.Buttons))
		for _, btn := range p.Buttons {
			rows = append(rows, btn.Label+" "+btn.Data)
		}
		markups = append(markups, strings.Join(rows, "\n"))
	}
	return markups
}

func (t *testServices) InProgress(userID uint64) (bool, error) {
	return t.engineActive, nil
}

func (t *testServices) FindByID(userID uint64) (*user.User, error) {
	if u, ok := t.usersBy[userID]; ok {
		return u, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (t *testServices) FindByCategoryAndSubcategory(userID uint64, category, name string) (*subcategory.Subcategory, error) {
	return nil, subcategory.ErrSubcategoryNotFound
}

func (t *testServices) CountBySubcategory(userID uint64, subcategoryID uint64) (int64, error) {
	return 0, nil
}

func (t *testServices) CountByDayForUser(userID uint64, from, to time.Time) ([]movement.DayCount, error) {
	return t.dayCounts, nil
}

func (t *testServices) CountForUser(userID uint64) (int64, error) {
	return t.counts, nil
}

func (t *testServices) SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error) {
	if t.sumRows != nil {
		return t.sumRows(q, groupBy)
	}
	return nil, nil
}

func (t *testServices) SumAmountForAccount(accountID uint64) (decimal.Decimal, error) {
	return decimal.Zero, nil
}

func (t *testServices) SentKeys(userID uint64) ([]string, error) {
	var keys []string
	for k, ok := range t.sentKeys {
		if ok {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (t *testServices) LastSentAt(userID uint64) (*time.Time, error) {
	ts, ok := t.lastSentAt[userID]
	if !ok {
		return nil, nil
	}
	return &ts, nil
}

func (t *testServices) MarkSent(userID uint64, key string) error {
	t.marked = append(t.marked, key)
	if t.sentKeys == nil {
		t.sentKeys = map[string]bool{}
	}
	t.sentKeys[key] = true
	return nil
}

func (t *testServices) MarkSentAgain(userID uint64, key string) error {
	return t.MarkSent(userID, key)
}

func (t *testServices) MarkTapped(userID uint64, key string) error {
	t.tapped = append(t.tapped, key)
	return nil
}

func (t *testServices) HandleQuery(ctx context.Context, chat messenger.Chat, userID uint64, text string) (bool, error) {
	t.asked = text
	return true, nil
}

func TestMaybeNudge_FiresCorrectTipAfterFirstMovement(t *testing.T) {
	svc := &testServices{counts: 1}

	chat := &messenger.FakeChat{}
	svc.tips().Maybe(context.Background(), chat, 1)

	if len(sentTexts(chat)) != 1 {
		t.Fatalf("expected 1 nudge sent, got %d: %+v", len(sentTexts(chat)), sentTexts(chat))
	}
	if !svc.sentKeys[correctTip] {
		t.Error("expected correct_tip marked sent")
	}
}

func TestMaybeNudge_SkipsWhenFlowInProgress(t *testing.T) {
	svc := &testServices{counts: 1, engineActive: true}

	chat := &messenger.FakeChat{}
	svc.tips().Maybe(context.Background(), chat, 1)

	if len(sentTexts(chat)) != 0 {
		t.Fatalf("expected no nudge while a flow is in progress, got %+v", sentTexts(chat))
	}
}

func TestMaybeNudge_OnceEver(t *testing.T) {
	svc := &testServices{counts: 1, sentKeys: map[string]bool{correctTip: true}}

	chat := &messenger.FakeChat{}
	svc.tips().Maybe(context.Background(), chat, 1)

	if len(sentTexts(chat)) != 0 {
		t.Fatalf("expected no nudge (correct_tip already sent once-ever, nothing else eligible), got %+v", sentTexts(chat))
	}
}

func TestMaybeNudge_QuestionTipCarriesButton(t *testing.T) {
	svc := &testServices{counts: 1}

	original := nudges
	t.Cleanup(func() { nudges = original })
	nudges = []nudgeDef{{
		key:      "test_question_tip",
		when:     func(_ *Tips, userID uint64, stats *nudgeStats) bool { return true },
		text:     "💡 Probando.",
		question: "¿Cuánto gasté esta semana?",
	}}

	chat := &messenger.FakeChat{}
	svc.tips().Maybe(context.Background(), chat, 1)

	if len(sentTexts(chat)) != 1 {
		t.Fatalf("expected 1 nudge sent, got %d: %+v", len(sentTexts(chat)), sentTexts(chat))
	}
	if !strings.Contains(sentMarkups(chat)[0], "nudge_q:test_question_tip") {
		t.Errorf("expected the button callback_data, got markup: %s", sentMarkups(chat)[0])
	}
	if !strings.Contains(sentMarkups(chat)[0], "gasté esta semana") {
		t.Errorf("expected the question as the button label, got markup: %s", sentMarkups(chat)[0])
	}
}

func TestNudgeCallbackDataFitsTelegramLimit(t *testing.T) {
	for _, n := range nudges {
		if n.question == "" {
			continue
		}
		if got := len(nudgeQueryPrefix + n.key); got > 64 {
			t.Errorf("callback_data for %q is %d bytes, over the 64-byte limit", n.key, got)
		}
	}
}

func TestHandleNudgeQuery_RunsTheQuestionAndSealsTheTap(t *testing.T) {
	svc := &testServices{queryAnswer: "Gastaste $5.000."}

	handled := svc.tips().HandleCallback(context.Background(), &messenger.FakeChat{}, 1, nudgeQueryPrefix+nudgeQueryTip)

	if !handled {
		t.Fatal("expected the nudge callback to be handled")
	}
	if svc.asked != nudgeQuestion(nudgeQueryTip) {
		t.Errorf("asked %q, want the tip's question %q", svc.asked, nudgeQuestion(nudgeQueryTip))
	}
	if len(svc.tapped) != 1 || svc.tapped[0] != nudgeQueryTip {
		t.Errorf("expected the tap sealed for %q, got %+v", nudgeQueryTip, svc.tapped)
	}
}

func TestHandleNudgeQuery_IgnoresOtherCallbacks(t *testing.T) {
	svc := &testServices{}

	if svc.tips().HandleCallback(context.Background(), &messenger.FakeChat{}, 1, "edit_proposal") {
		t.Error("a non-nudge callback must not be handled here — the engine owns it")
	}
	if svc.tips().HandleCallback(context.Background(), &messenger.FakeChat{}, 1, nudgeQueryPrefix+"key_que_no_existe") {
		t.Error("an unknown nudge key must not fire a query")
	}
	if svc.asked != "" {
		t.Errorf("no query should have run, but asked %q", svc.asked)
	}
}

func TestMaybeNudge_DailyCooldown(t *testing.T) {
	svc := &testServices{counts: 1, lastSentAt: map[uint64]time.Time{1: time.Now()}}

	chat := &messenger.FakeChat{}
	svc.tips().Maybe(context.Background(), chat, 1)

	if len(sentTexts(chat)) != 0 {
		t.Fatalf("expected no nudge inside the daily cooldown, got %+v", sentTexts(chat))
	}
}
