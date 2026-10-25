package messaging

import (
	"context"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
)

func (c *controller) finishCategoryMatchOffer(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	c.finisher().FinishCategoryMatchOffer(ctx, chat, data)
}

func (c *controller) finishCategoryProposalConfirm(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	c.finisher().FinishCategoryProposalConfirm(ctx, chat, data)
}
