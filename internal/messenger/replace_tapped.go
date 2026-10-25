package messenger

import (
	"context"
	"log/slog"

	"lopiibot.com/internal/conversation"
)

type tappedChat struct {
	Chat
	replaced bool
}

func ReplaceTapped(c Chat) Chat {
	return &tappedChat{Chat: c}
}

func (t *tappedChat) Send(ctx context.Context, p conversation.Prompt) error {
	if t.replaced {
		return t.Chat.Send(ctx, p)
	}
	t.replaced = true
	if err := EditSource(ctx, t.Chat, p); err != nil {
		slog.WarnContext(ctx, "messenger: edit of the tapped message failed, sending it new", "err", err)
		return t.Chat.Send(ctx, p)
	}
	return nil
}

func (t *tappedChat) EditSource(ctx context.Context, p conversation.Prompt) error {
	t.replaced = true
	return EditSource(ctx, t.Chat, p)
}

func (t *tappedChat) ClearSourceButtons(ctx context.Context) error {
	return ClearSourceButtons(ctx, t.Chat)
}
