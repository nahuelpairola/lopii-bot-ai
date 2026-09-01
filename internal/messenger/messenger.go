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
