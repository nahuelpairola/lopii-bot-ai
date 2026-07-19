package messaging

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
)

// stubNudgeRepo mocks nudgeRepository with controllable WasSent/LastSentAt.
type stubNudgeRepo struct {
	sent     map[string]bool
	lastSent map[uint64]time.Time
	marked   []string
}

func (r *stubNudgeRepo) WasSent(userID uint64, key string) (bool, error) {
	return r.sent[key], nil
}

func (r *stubNudgeRepo) MarkSent(userID uint64, key string) error {
	r.marked = append(r.marked, key)
	if r.sent == nil {
		r.sent = map[string]bool{}
	}
	r.sent[key] = true
	return nil
}

func (r *stubNudgeRepo) LastSentAt(userID uint64) (*time.Time, error) {
	t, ok := r.lastSent[userID]
	if !ok {
		return nil, nil
	}
	return &t, nil
}

func newNudgeTestBot(t *testing.T) (*bot.Bot, *recordingTransport) {
	t.Helper()
	rt := &recordingTransport{}
	b, err := bot.New("123:ABC", bot.WithSkipGetMe(), bot.WithHTTPClient(time.Second, &http.Client{Transport: rt}))
	if err != nil {
		t.Fatalf("bot.New: %v", err)
	}
	return b, rt
}

func TestMaybeNudge_FiresCorrectTipAfterFirstMovement(t *testing.T) {
	movRepo := &fakeMovementRepoFull{countForUser: 1}
	nudgeRepo := &stubNudgeRepo{}
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	c := &controller{movements: movRepo, reminders: &fakeReminderRepo{}, users: &fakeUserRepository{}, accounts: &fakeAccountRepoFull{}, nudges: nudgeRepo, engine: engine}

	b, rt := newNudgeTestBot(t)
	c.maybeNudge(context.Background(), b, 1, 1)

	if len(rt.texts) != 1 {
		t.Fatalf("expected 1 nudge sent, got %d: %+v", len(rt.texts), rt.texts)
	}
	if !nudgeRepo.sent[nudgeCorrectTip] {
		t.Error("expected correct_tip marked sent")
	}
}

func TestMaybeNudge_SkipsWhenFlowInProgress(t *testing.T) {
	movRepo := &fakeMovementRepoFull{countForUser: 1}
	nudgeRepo := &stubNudgeRepo{}
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(NewAccountCreateFlow())
	if _, err := engine.Start(1, accountCreateFlowName); err != nil {
		t.Fatalf("engine.Start: %v", err)
	}
	c := &controller{movements: movRepo, nudges: nudgeRepo, engine: engine}

	b, rt := newNudgeTestBot(t)
	c.maybeNudge(context.Background(), b, 1, 1)

	if len(rt.texts) != 0 {
		t.Fatalf("expected no nudge while a flow is in progress, got %+v", rt.texts)
	}
}

func TestMaybeNudge_OnceEver(t *testing.T) {
	movRepo := &fakeMovementRepoFull{countForUser: 1}
	nudgeRepo := &stubNudgeRepo{sent: map[string]bool{nudgeCorrectTip: true}}
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	c := &controller{movements: movRepo, reminders: &fakeReminderRepo{}, users: &fakeUserRepository{}, accounts: &fakeAccountRepoFull{}, nudges: nudgeRepo, engine: engine}

	b, rt := newNudgeTestBot(t)
	c.maybeNudge(context.Background(), b, 1, 1)

	if len(rt.texts) != 0 {
		t.Fatalf("expected no nudge (correct_tip already sent once-ever, nothing else eligible), got %+v", rt.texts)
	}
}

func TestMaybeNudge_DailyCooldown(t *testing.T) {
	movRepo := &fakeMovementRepoFull{countForUser: 1}
	nudgeRepo := &stubNudgeRepo{lastSent: map[uint64]time.Time{1: time.Now()}}
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	c := &controller{movements: movRepo, nudges: nudgeRepo, engine: engine}

	b, rt := newNudgeTestBot(t)
	c.maybeNudge(context.Background(), b, 1, 1)

	if len(rt.texts) != 0 {
		t.Fatalf("expected no nudge inside the daily cooldown, got %+v", rt.texts)
	}
}
