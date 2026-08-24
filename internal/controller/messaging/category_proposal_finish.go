package messaging

import (
	"context"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
)

// finishCategoryMatchOffer y finishCategoryProposalConfirm delegan en flow
// (category_finish.go). Conservan los nombres de borde mientras los tests y
// handleFlowFinished los usen.
func (c *controller) finishCategoryMatchOffer(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	flow.FinishCategoryMatchOffer(ctx, c, chat, data)
}

func (c *controller) finishCategoryProposalConfirm(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	flow.FinishCategoryProposalConfirm(ctx, c, chat, data)
}
