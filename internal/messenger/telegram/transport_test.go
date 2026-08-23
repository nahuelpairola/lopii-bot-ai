package telegram

import (
	"testing"

	"github.com/go-telegram/bot/models"
	"lopiibot.com/internal/user"
)

func TestToIncoming_TextMessage(t *testing.T) {
	u := &models.Update{Message: &models.Message{
		Text: "gasté 3000 en el súper",
		From: &models.User{ID: 777, Username: "nahue"},
		Chat: models.Chat{ID: 555},
	}}
	in, ok := toIncoming(u, nil)
	if !ok {
		t.Fatal("un mensaje de texto tiene que traducirse")
	}
	if in.Channel != user.ChannelTelegram {
		t.Errorf("Channel = %q, want %q", in.Channel, user.ChannelTelegram)
	}
	if in.ChannelUserID != "777" {
		t.Errorf("ChannelUserID = %q, want 777", in.ChannelUserID)
	}
	if in.DisplayName != "nahue" {
		t.Errorf("DisplayName = %q, want nahue", in.DisplayName)
	}
	if in.Input.Text != "gasté 3000 en el súper" {
		t.Errorf("Input.Text = %q", in.Input.Text)
	}
	if in.Input.CallbackData != "" {
		t.Error("un mensaje de texto no trae CallbackData")
	}
}

func TestToIncoming_Callback(t *testing.T) {
	u := &models.Update{CallbackQuery: &models.CallbackQuery{
		ID:   "cb1",
		Data: "yes",
		From: models.User{ID: 777},
		Message: models.MaybeInaccessibleMessage{
			Message: &models.Message{Chat: models.Chat{ID: 555}},
		},
	}}
	in, ok := toIncoming(u, nil)
	if !ok {
		t.Fatal("un callback tiene que traducirse")
	}
	if in.Input.CallbackData != "yes" {
		t.Errorf("CallbackData = %q, want yes", in.Input.CallbackData)
	}
	if in.Input.Text != "" {
		t.Error("un callback no trae Text")
	}
}

// Un comando NO es input de conversación: /start lo maneja el adapter aparte
// (canje de invitación por deep-link), no la superficie neutra.
func TestToIncoming_CommandIsNotConversationInput(t *testing.T) {
	u := &models.Update{Message: &models.Message{
		Text: "/start ABC123",
		From: &models.User{ID: 777},
		Chat: models.Chat{ID: 555},
	}}
	if _, ok := toIncoming(u, nil); ok {
		t.Error("/start no tiene que llegar al handler neutro")
	}
}

func TestToIncoming_EmptyUpdateIsSkipped(t *testing.T) {
	if _, ok := toIncoming(&models.Update{}, nil); ok {
		t.Error("un update vacío no se traduce")
	}
}
