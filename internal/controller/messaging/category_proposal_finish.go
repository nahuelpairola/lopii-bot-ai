package messaging

import (
	"context"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
)

// finishCategoryMatchOffer y finishCategoryProposalConfirm delegan en flow
// (category_finish.go). Conservan los nombres de borde mientras los tests y
// handleFlowFinished los usen.
func (c *controller) finishCategoryMatchOffer(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	flow.FinishCategoryMatchOffer(ctx, c, b, chatID, data)
}

func (c *controller) finishCategoryProposalConfirm(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	flow.FinishCategoryProposalConfirm(ctx, c, b, chatID, data)
}
