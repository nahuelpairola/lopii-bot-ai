package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/user"
)

type AddressLookup interface {
	FindChannelID(userID uint64, channel string) (string, error)
}

type Transport struct {
	b        *bot.Bot
	addrs    AddressLookup
	baseHost string
}

func New(b *bot.Bot, addrs AddressLookup, baseHost string) *Transport {
	return &Transport{b: b, addrs: addrs, baseHost: baseHost}
}

func (t *Transport) ChatFor(userID uint64) (messenger.Chat, error) {
	addr, err := t.addrs.FindChannelID(userID, user.ChannelTelegram)
	if err != nil {
		return nil, err
	}
	chatID, err := strconv.ParseInt(addr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("telegram: dirección inválida %q: %w", addr, err)
	}
	return chat{b: t.b, chatID: chatID, baseHost: t.baseHost}, nil
}

func (t *Transport) Serve(h messenger.Handler) http.Handler {
	t.b.RegisterHandlerMatchFunc(
		func(u *models.Update) bool { _, ok := toIncoming(u, t.b); return ok },
		func(ctx context.Context, b *bot.Bot, u *models.Update) {
			in, ok := toIncoming(u, b)
			if !ok {
				return
			}
			if cb := u.CallbackQuery; cb != nil && b != nil {
				_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
			}
			h(ctx, in)
		},
	)
	return t.b.WebhookHandler()
}

func (t *Transport) RegisterCommand(cmd string, fn bot.HandlerFunc) {
	t.b.RegisterHandler(bot.HandlerTypeMessageText, cmd, bot.MatchTypePrefix, fn)
}

func toIncoming(u *models.Update, b *bot.Bot) (messenger.Incoming, bool) {
	switch {
	case u.CallbackQuery != nil:
		cb := u.CallbackQuery
		var chatID int64
		switch {
		case cb.Message.Message != nil:
			chatID = cb.Message.Message.Chat.ID
		case cb.Message.InaccessibleMessage != nil:
			chatID = cb.Message.InaccessibleMessage.Chat.ID
		}
		if chatID == 0 {
			slog.Warn("telegram: callback sin chat resoluble, se descarta", "callback_id", cb.ID)
			return messenger.Incoming{}, false
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
		if m.Chat.ID == 0 {
			slog.Warn("telegram: mensaje sin chat resoluble, se descarta")
			return messenger.Incoming{}, false
		}
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
