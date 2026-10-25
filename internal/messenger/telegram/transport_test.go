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

func TestToIncoming_CallbackRemembersTheTappedMessage(t *testing.T) {
	for name, msg := range map[string]models.MaybeInaccessibleMessage{
		"accessible":   {Message: &models.Message{ID: 99, Chat: models.Chat{ID: 555}}},
		"inaccessible": {InaccessibleMessage: &models.InaccessibleMessage{MessageID: 99, Chat: models.Chat{ID: 555}}},
	} {
		u := &models.Update{CallbackQuery: &models.CallbackQuery{ID: "cb1", Data: "x", From: models.User{ID: 777}, Message: msg}}
		in, ok := toIncoming(u, nil)
		if !ok {
			t.Fatalf("%s: not translated", name)
		}
		if got := in.Chat.(chat).sourceMessageID; got != 99 {
			t.Errorf("%s: sourceMessageID = %d, want 99 — without it the tap cannot edit its own message", name, got)
		}
	}
}
