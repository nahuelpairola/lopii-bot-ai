package messaging

import (
	"context"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
)

func (c *controller) finishCategoryMatchOffer(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	flow.FinishCategoryMatchOffer(ctx, c, chat, data)
}

func (c *controller) finishCategoryProposalConfirm(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	flow.FinishCategoryProposalConfirm(ctx, c, chat, data)
}
