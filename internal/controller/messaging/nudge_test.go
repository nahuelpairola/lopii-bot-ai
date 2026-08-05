package messaging

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/orchestrator"
)

// stubNudgeRepo mocks nudgeRepository with controllable SentKeys/LastSentAt.
type stubNudgeRepo struct {
	sent     map[string]bool
	lastSent map[uint64]time.Time
	marked   []string
	tapped   []string
}

func (r *stubNudgeRepo) WasSent(userID uint64, key string) (bool, error) {
	return r.sent[key], nil
}

func (r *stubNudgeRepo) SentKeys(userID uint64) ([]string, error) {
	var keys []string
	for k, ok := range r.sent {
		if ok {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (r *stubNudgeRepo) MarkSent(userID uint64, key string) error {
	r.marked = append(r.marked, key)
	if r.sent == nil {
		r.sent = map[string]bool{}
	}
	r.sent[key] = true
	return nil
}

func (r *stubNudgeRepo) MarkSentAgain(userID uint64, key string) error {
	return r.MarkSent(userID, key)
}

func (r *stubNudgeRepo) MarkTapped(userID uint64, key string) error {
	r.tapped = append(r.tapped, key)
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

// Un tip con question sale con un botón cuyo callback_data es el prefijo + la
// key, y cuyo label ES la pregunta: así el usuario aprende la frase que
// después puede escribir solo.
func TestMaybeNudge_QuestionTipCarriesButton(t *testing.T) {
	movRepo := &fakeMovementRepoFull{countForUser: 1}
	nudgeRepo := &stubNudgeRepo{}
	store := &fakeStoreForController{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	c := &controller{movements: movRepo, reminders: &fakeReminderRepo{}, users: &fakeUserRepository{}, accounts: &fakeAccountRepoFull{}, nudges: nudgeRepo, engine: engine}

	original := nudges
	t.Cleanup(func() { nudges = original })
	nudges = []nudgeDef{{
		key:      "test_question_tip",
		when:     func(c *controller, userID uint64, s *nudgeStats) bool { return true },
		text:     "💡 Probando.",
		question: "¿Cuánto gasté esta semana?",
	}}

	b, rt := newNudgeTestBot(t)
	c.maybeNudge(context.Background(), b, 1, 1)

	if len(rt.texts) != 1 {
		t.Fatalf("expected 1 nudge sent, got %d: %+v", len(rt.texts), rt.texts)
	}
	if !strings.Contains(rt.markups[0], "nudge_q:test_question_tip") {
		t.Errorf("expected the button callback_data, got markup: %s", rt.markups[0])
	}
	if !strings.Contains(rt.markups[0], "gasté esta semana") {
		t.Errorf("expected the question as the button label, got markup: %s", rt.markups[0])
	}
}

// Telegram trunca callback_data pasados los 64 bytes y el botón deja de
// matchear en silencio.
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

// stubQueryOrchestrator: solo AnswerQuery importa acá. El resto del
// movementOrchestrator queda embebido en nil — si el código bajo test llamara
// a cualquier otro método, el panic señala exactamente eso.
type stubQueryOrchestrator struct {
	movementOrchestrator
	asked  string
	answer string
}

func (o *stubQueryOrchestrator) AnswerQuery(ctx context.Context, systemPrompt, userText string, history []orchestrator.QueryTurn, tools []orchestrator.AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error) {
	o.asked = userText
	return o.answer, nil
}

func TestHandleNudgeQuery_RunsTheQuestionAndSealsTheTap(t *testing.T) {
	orch := &stubQueryOrchestrator{answer: "Gastaste $5.000."}
	nudgeRepo := &stubNudgeRepo{}
	c := &controller{orchestrator: orch, nudges: nudgeRepo, chatHistory: stubChatHistory{}}

	handled := c.handleNudgeQuery(context.Background(), nil, 1, 1, nudgeQueryPrefix+nudgeQueryTip)

	if !handled {
		t.Fatal("expected the nudge callback to be handled")
	}
	if orch.asked != nudgeQuestion(nudgeQueryTip) {
		t.Errorf("asked %q, want the tip's question %q", orch.asked, nudgeQuestion(nudgeQueryTip))
	}
	if len(nudgeRepo.tapped) != 1 || nudgeRepo.tapped[0] != nudgeQueryTip {
		t.Errorf("expected the tap sealed for %q, got %+v", nudgeQueryTip, nudgeRepo.tapped)
	}
}

func TestHandleNudgeQuery_IgnoresOtherCallbacks(t *testing.T) {
	orch := &stubQueryOrchestrator{}
	c := &controller{orchestrator: orch, nudges: &stubNudgeRepo{}, chatHistory: stubChatHistory{}}

	if c.handleNudgeQuery(context.Background(), nil, 1, 1, "edit_proposal") {
		t.Error("a non-nudge callback must not be handled here — the engine owns it")
	}
	if c.handleNudgeQuery(context.Background(), nil, 1, 1, nudgeQueryPrefix+"key_que_no_existe") {
		t.Error("an unknown nudge key must not fire a query")
	}
	if orch.asked != "" {
		t.Errorf("no query should have run, but asked %q", orch.asked)
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
