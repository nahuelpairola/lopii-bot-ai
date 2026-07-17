package messaging

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/user"
)

// stubTransport answers every Telegram API call with ok:true so the handler's
// AnswerCallbackQuery/EditMessageText calls don't hit the network in tests.
type stubTransport struct{}

func (stubTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"ok":true,"result":true}`))),
		Header:     make(http.Header),
	}, nil
}

func testBot(t *testing.T) *bot.Bot {
	t.Helper()
	b, err := bot.New("123:ABC", bot.WithSkipGetMe(),
		bot.WithHTTPClient(time.Second, &http.Client{Transport: stubTransport{}}))
	if err != nil {
		t.Fatalf("bot.New: %v", err)
	}
	return b
}

func TestHandleWeeklySummaryOff_DisablesWeekly(t *testing.T) {
	rem := &fakeReminderRepo{}
	c := &controller{
		users:     &fakeUserRepository{byTelegramID: map[string]*user.User{"99": {ID: 7, TelegramID: "99"}}},
		reminders: rem,
	}

	update := &models.Update{CallbackQuery: &models.CallbackQuery{
		ID:   "cbid",
		Data: constants.WeeklySummaryOffData,
		From: models.User{ID: 99},
		Message: models.MaybeInaccessibleMessage{Message: &models.Message{
			ID:   1,
			Chat: models.Chat{ID: 99},
		}},
	}}

	c.handleWeeklySummaryOff(context.Background(), testBot(t), update)

	if rem.weeklySet == nil || *rem.weeklySet != false || rem.weeklySetFor != 7 {
		t.Fatalf("expected SetWeeklySummary(7,false), got for=%d val=%v", rem.weeklySetFor, rem.weeklySet)
	}
}
