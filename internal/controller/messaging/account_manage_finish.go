package messaging

import (
	"context"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
)

func (c *controller) finishAccountManageFlow(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	flow.FinishAccountManage(ctx, c, chat, data)
}

func (c *controller) finishAccountAdjust(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	flow.FinishAccountAdjust(ctx, c, chat, data)
}

func (c *controller) finishAccountDefault(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	flow.FinishAccountDefault(ctx, c, chat, data)
}

func (c *controller) finishAccountMoveOffer(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	flow.FinishAccountMoveOffer(ctx, c, chat, data)
}
