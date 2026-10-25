package messaging

import (
	"context"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
)

func (c *controller) finishAccountManageFlow(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	c.finisher().FinishAccountManage(ctx, chat, data)
}

func (c *controller) finishAccountAdjust(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	c.finisher().FinishAccountAdjust(ctx, chat, data)
}

func (c *controller) finishAccountDefault(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	c.finisher().FinishAccountDefault(ctx, chat, data)
}

func (c *controller) finishAccountMoveOffer(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	c.finisher().FinishAccountMoveOffer(ctx, chat, data)
}
