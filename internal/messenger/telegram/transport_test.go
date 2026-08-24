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
	if got := in.Chat.(chat).chatID; got != 555 {
		t.Errorf("chatID = %d, want 555 (el chat, no el sender 777)", got)
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
	if got := in.Chat.(chat).chatID; got != 555 {
		t.Errorf("chatID = %d, want 555 (el chat, no el sender 777)", got)
	}
}

// Un callback sobre un mensaje inaccesible (borrado, o demasiado viejo) trae
// Message.Message == nil, pero Message.InaccessibleMessage sigue trayendo el
// Chat: hay que recuperar el chatID de ahí, no perder el callback.
func TestToIncoming_CallbackOnInaccessibleMessage(t *testing.T) {
	u := &models.Update{CallbackQuery: &models.CallbackQuery{
		ID:   "cb1",
		Data: "yes",
		From: models.User{ID: 777},
		Message: models.MaybeInaccessibleMessage{
			InaccessibleMessage: &models.InaccessibleMessage{Chat: models.Chat{ID: 555}},
		},
	}}
	in, ok := toIncoming(u, nil)
	if !ok {
		t.Fatal("un callback sobre un mensaje inaccesible igual tiene que traducirse, el Chat está en InaccessibleMessage")
	}
	if got := in.Chat.(chat).chatID; got != 555 {
		t.Errorf("chatID = %d, want 555", got)
	}
}

// Si ninguno de los dos brazos de la unión trae un chat resoluble, no hay que
// devolver un Incoming con chatID 0 (mandaría al chat 0, en silencio):
// se descarta el update.
func TestToIncoming_CallbackWithNoResolvableChatIsDropped(t *testing.T) {
	u := &models.Update{CallbackQuery: &models.CallbackQuery{
		ID:   "cb1",
		Data: "yes",
		From: models.User{ID: 777},
	}}
	if _, ok := toIncoming(u, nil); ok {
		t.Error("un callback sin chat resoluble no tiene que traducirse")
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
