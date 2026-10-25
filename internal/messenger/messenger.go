package messenger

import (
	"context"

	"lopiibot.com/internal/conversation"
)

type Chat interface {
	Send(ctx context.Context, p conversation.Prompt) error
	Typing(ctx context.Context) error
}

type Incoming struct {
	Channel       string
	ChannelUserID string
	DisplayName   string
	Chat          Chat
	Input         conversation.Input
}

type Handler func(ctx context.Context, in Incoming)

func SendText(ctx context.Context, c Chat, s string) error {
	return c.Send(ctx, conversation.Prompt{Text: s})
}

type SourceEditor interface {
	EditSource(ctx context.Context, p conversation.Prompt) error
	ClearSourceButtons(ctx context.Context) error
}

func EditSource(ctx context.Context, c Chat, p conversation.Prompt) error {
	if e, ok := c.(SourceEditor); ok {
		return e.EditSource(ctx, p)
	}
	return c.Send(ctx, p)
}

func ClearSourceButtons(ctx context.Context, c Chat) error {
	if e, ok := c.(SourceEditor); ok {
		return e.ClearSourceButtons(ctx)
	}
	return nil
}
