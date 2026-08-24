package nudges

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
	"lopiibot.com/internal/user"
)

// testServices es el test-double de Services para el dispatcher: campos
// configurables para lo que cada test programa y registros (texts, markups,
// marked, tapped, asked) para lo que aserción después. Métodos que un test no
// programa devuelven cero con nil-error, espejando los fakes de messaging que
// usaba el cluster.
type testServices struct {
	counts    int64
	dayCounts []movement.DayCount
	// sumRows: SumForUser del repo. Sin programar, devuelve cero filas.
	sumRows func(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error)
	// accountsBy: FindByUserID del repo.
	accountsBy []account.Account
	// remindersBy: FindByUserID del repo, indexado por userID.
	remindersBy map[uint64]*reminder.Reminder
	// usersBy: FindByID del repo, indexado por userID.
	usersBy map[uint64]*user.User
	// engineActive simula un flow en curso en el engine.
	engineActive bool
	// available gobierna NudgesAvailable. Por defecto (nil) el storage de
	// nudges está cableado, como en los tests de messaging que usaba el cluster.
	available *bool
	// sentKeys: las keys ya enviadas. NudgesMarkSent las marca acá.
	sentKeys map[string]bool
	// lastSentAt: el timestamp del último nudge, por userID.
	lastSentAt map[uint64]time.Time
	marked     []string
	tapped     []string
	// asked: el texto que HandleQuery recibió.
	asked string
	// queryAnswer: vestigial del stubQueryOrchestrator — el loop de QUERY
	// siempre contestaba con answered=true.
	queryAnswer string
	texts       []string
	markups     []string
}

func (t *testServices) EngineInProgress(userID uint64) (bool, error) {
	return t.engineActive, nil
}

func (t *testServices) RemindersFindByUserID(userID uint64) (*reminder.Reminder, error) {
	if r, ok := t.remindersBy[userID]; ok {
		return r, nil
	}
	return nil, nil
}

func (t *testServices) UsersFindByID(userID uint64) (*user.User, error) {
	if u, ok := t.usersBy[userID]; ok {
		return u, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (t *testServices) AccountsFindByUserID(userID uint64) ([]account.Account, error) {
	return t.accountsBy, nil
}

func (t *testServices) SubcategoriesFindByCategoryAndSubcategory(userID uint64, category, name string) (*subcategory.Subcategory, error) {
	return nil, subcategory.ErrSubcategoryNotFound
}

func (t *testServices) MovementsCountBySubcategory(userID uint64, subcategoryID uint64) (int64, error) {
	return 0, nil
}

func (t *testServices) MovementsCountByDayForUser(userID uint64, from, to time.Time) ([]movement.DayCount, error) {
	return t.dayCounts, nil
}

func (t *testServices) MovementsCountForUser(userID uint64) (int64, error) {
	return t.counts, nil
}

func (t *testServices) MovementsSumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error) {
	if t.sumRows != nil {
		return t.sumRows(q, groupBy)
	}
	return nil, nil
}

func (t *testServices) MovementsSumAmountForAccount(accountID uint64) (decimal.Decimal, error) {
	return decimal.Zero, nil
}

func (t *testServices) NudgesAvailable() bool {
	return t.available == nil || *t.available
}

func (t *testServices) NudgesSentKeys(userID uint64) ([]string, error) {
	var keys []string
	for k, ok := range t.sentKeys {
		if ok {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (t *testServices) NudgesLastSentAt(userID uint64) (*time.Time, error) {
	ts, ok := t.lastSentAt[userID]
	if !ok {
		return nil, nil
	}
	return &ts, nil
}

func (t *testServices) NudgesMarkSent(userID uint64, key string) error {
	t.marked = append(t.marked, key)
	if t.sentKeys == nil {
		t.sentKeys = map[string]bool{}
	}
	t.sentKeys[key] = true
	return nil
}

func (t *testServices) NudgesMarkSentAgain(userID uint64, key string) error {
	return t.NudgesMarkSent(userID, key)
}

func (t *testServices) NudgesMarkTapped(userID uint64, key string) error {
	t.tapped = append(t.tapped, key)
	return nil
}

func (t *testServices) SendText(ctx context.Context, chat messenger.Chat, text string) {
	t.texts = append(t.texts, text)
}

// SendPrompt replica el render de recordingTransport (movement_create_flow_test):
// cada botón como "label data", así los tests pueden hacer strings.Contains sobre
// el label Y el callback data.
func (t *testServices) SendPrompt(ctx context.Context, chat messenger.Chat, prompt conversation.Prompt) {
	t.texts = append(t.texts, prompt.Text)
	rows := make([]string, 0, len(prompt.Buttons))
	for _, btn := range prompt.Buttons {
		rows = append(rows, btn.Label+" "+btn.Data)
	}
	t.markups = append(t.markups, strings.Join(rows, "\n"))
}

func (t *testServices) HandleQuery(ctx context.Context, chat messenger.Chat, userID uint64, text string) (bool, error) {
	t.asked = text
	return true, nil
}

func TestMaybeNudge_FiresCorrectTipAfterFirstMovement(t *testing.T) {
	svc := &testServices{counts: 1}

	chat := &messenger.FakeChat{}
	Maybe(context.Background(), svc, chat, 1)

	if len(svc.texts) != 1 {
		t.Fatalf("expected 1 nudge sent, got %d: %+v", len(svc.texts), svc.texts)
	}
	if !svc.sentKeys[correctTip] {
		t.Error("expected correct_tip marked sent")
	}
}

func TestMaybeNudge_SkipsWhenFlowInProgress(t *testing.T) {
	svc := &testServices{counts: 1, engineActive: true}

	chat := &messenger.FakeChat{}
	Maybe(context.Background(), svc, chat, 1)

	if len(svc.texts) != 0 {
		t.Fatalf("expected no nudge while a flow is in progress, got %+v", svc.texts)
	}
}

func TestMaybeNudge_OnceEver(t *testing.T) {
	svc := &testServices{counts: 1, sentKeys: map[string]bool{correctTip: true}}

	chat := &messenger.FakeChat{}
	Maybe(context.Background(), svc, chat, 1)

	if len(svc.texts) != 0 {
		t.Fatalf("expected no nudge (correct_tip already sent once-ever, nothing else eligible), got %+v", svc.texts)
	}
}

// Un tip con question sale con un botón cuyo callback_data es el prefijo + la
// key, y cuyo label ES la pregunta: así el usuario aprende la frase que
// después puede escribir solo.
func TestMaybeNudge_QuestionTipCarriesButton(t *testing.T) {
	svc := &testServices{counts: 1}

	original := nudges
	t.Cleanup(func() { nudges = original })
	nudges = []nudgeDef{{
		key:      "test_question_tip",
		when:     func(s Services, userID uint64, stats *nudgeStats) bool { return true },
		text:     "💡 Probando.",
		question: "¿Cuánto gasté esta semana?",
	}}

	chat := &messenger.FakeChat{}
	Maybe(context.Background(), svc, chat, 1)

	if len(svc.texts) != 1 {
		t.Fatalf("expected 1 nudge sent, got %d: %+v", len(svc.texts), svc.texts)
	}
	if !strings.Contains(svc.markups[0], "nudge_q:test_question_tip") {
		t.Errorf("expected the button callback_data, got markup: %s", svc.markups[0])
	}
	if !strings.Contains(svc.markups[0], "gasté esta semana") {
		t.Errorf("expected the question as the button label, got markup: %s", svc.markups[0])
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

func TestHandleNudgeQuery_RunsTheQuestionAndSealsTheTap(t *testing.T) {
	svc := &testServices{queryAnswer: "Gastaste $5.000."}

	handled := HandleCallback(context.Background(), svc, &messenger.FakeChat{}, 1, nudgeQueryPrefix+nudgeQueryTip)

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

	if HandleCallback(context.Background(), svc, &messenger.FakeChat{}, 1, "edit_proposal") {
		t.Error("a non-nudge callback must not be handled here — the engine owns it")
	}
	if HandleCallback(context.Background(), svc, &messenger.FakeChat{}, 1, nudgeQueryPrefix+"key_que_no_existe") {
		t.Error("an unknown nudge key must not fire a query")
	}
	if svc.asked != "" {
		t.Errorf("no query should have run, but asked %q", svc.asked)
	}
}

func TestMaybeNudge_DailyCooldown(t *testing.T) {
	svc := &testServices{counts: 1, lastSentAt: map[uint64]time.Time{1: time.Now()}}

	chat := &messenger.FakeChat{}
	Maybe(context.Background(), svc, chat, 1)

	if len(svc.texts) != 0 {
		t.Fatalf("expected no nudge inside the daily cooldown, got %+v", svc.texts)
	}
}
