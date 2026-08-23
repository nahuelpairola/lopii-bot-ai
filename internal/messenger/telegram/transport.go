package telegram

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/user"
)

// AddressLookup resuelve la dirección de un usuario en este canal. Es la
// interfaz local del adapter contra el repo de usuarios: acá no se importa
// ningún tipo concreto de repositorio.
type AddressLookup interface {
	FindChannelID(userID uint64, channel string) (string, error)
}

// Transport es el canal Telegram. Es el único lugar del árbol que conoce
// *bot.Bot fuera del arranque del server.
type Transport struct {
	b     *bot.Bot
	addrs AddressLookup
}

func New(b *bot.Bot, addrs AddressLookup) *Transport {
	return &Transport{b: b, addrs: addrs}
}

// ChatFor alcanza a un usuario que NO acaba de escribir: lo usan el sweeper de
// notifier y el drenaje de pendingjob. El strconv.ParseInt que estaba
// duplicado en tres sitios vive acá y sólo acá.
func (t *Transport) ChatFor(userID uint64) (messenger.Chat, error) {
	addr, err := t.addrs.FindChannelID(userID, user.ChannelTelegram)
	if err != nil {
		return nil, err
	}
	chatID, err := strconv.ParseInt(addr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("telegram: dirección inválida %q: %w", addr, err)
	}
	return chat{b: t.b, chatID: chatID}, nil
}

// Serve registra el handler catch-all en el bot y devuelve el http.Handler del
// webhook. El borde no vuelve a ver un *models.Update.
func (t *Transport) Serve(h messenger.Handler) http.Handler {
	t.b.RegisterHandlerMatchFunc(
		func(u *models.Update) bool { _, ok := toIncoming(u, t.b); return ok },
		func(ctx context.Context, b *bot.Bot, u *models.Update) {
			in, ok := toIncoming(u, b)
			if !ok {
				return
			}
			// El ack va acá adentro, antes del handler: apagar el reloj del
			// botón es cosa de Telegram y lopii no sabe que existen los
			// callbacks. Antes corría después de tomar el lock por usuario.
			if cb := u.CallbackQuery; cb != nil && b != nil {
				_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
			}
			h(ctx, in)
		},
	)
	return t.b.WebhookHandler()
}

// RegisterCommand cuelga un handler de comando. El onboarding (/start CODE) es
// un canje de invitación por deep-link de Telegram, no un concepto neutro, así
// que se registra por acá y no pasa por messenger.Handler.
func (t *Transport) RegisterCommand(cmd string, fn bot.HandlerFunc) {
	t.b.RegisterHandler(bot.HandlerTypeMessageText, cmd, bot.MatchTypePrefix, fn)
}

// toIncoming traduce un update a la superficie neutra. Devuelve false para
// todo lo que el motor de conversaciones no puede procesar: comandos (los toma
// RegisterCommand) y updates sin texto ni callback.
func toIncoming(u *models.Update, b *bot.Bot) (messenger.Incoming, bool) {
	switch {
	case u.CallbackQuery != nil:
		cb := u.CallbackQuery
		var chatID int64
		if cb.Message.Message != nil {
			chatID = cb.Message.Message.Chat.ID
		}
		return messenger.Incoming{
			Channel:       user.ChannelTelegram,
			ChannelUserID: fmt.Sprint(cb.From.ID),
			DisplayName:   cb.From.Username,
			Chat:          chat{b: b, chatID: chatID},
			Input:         conversation.Input{CallbackData: cb.Data},
		}, true

	case u.Message != nil && u.Message.Text != "" && !strings.HasPrefix(u.Message.Text, "/"):
		m := u.Message
		var name string
		if m.From != nil {
			name = m.From.Username
		}
		var from int64
		if m.From != nil {
			from = m.From.ID
		}
		return messenger.Incoming{
			Channel:       user.ChannelTelegram,
			ChannelUserID: fmt.Sprint(from),
			DisplayName:   name,
			Chat:          chat{b: b, chatID: m.Chat.ID},
			Input:         conversation.Input{Text: m.Text},
		}, true
	}
	return messenger.Incoming{}, false
}
