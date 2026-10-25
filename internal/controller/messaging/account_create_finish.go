package messaging

import (
	"context"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
)

func (c *controller) finishAccountCreateFlow(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	c.finisher().FinishAccountCreate(ctx, chat, data)
}
