package messaging

import (
	"context"

	"lopiibot.com/internal/agent"
	"lopiibot.com/internal/messenger"
)

func (c *controller) sendText(ctx context.Context, chat messenger.Chat, text string) {
	_ = messenger.SendText(ctx, chat, text)
}

func (c *controller) handleFreeText(ctx context.Context, chat messenger.Chat, userID uint64, text string) error {
	return agent.StartLoop(ctx, c, chat, userID, text)
}
