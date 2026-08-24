package messaging

import (
	"context"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
)

// finishSubcategorySetupFlow delega en flow.FinishSubcategorySetup
// (category_finish.go). Conserva los nombres de borde mientras los tests y
// handleFlowFinished los usen.
func (c *controller) finishSubcategorySetupFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	flow.FinishSubcategorySetup(ctx, c, newEdgeChat(b, chatID), data)
}

func (c *controller) insertNewSubcategory(data conversation.Data) error {
	return flow.InsertNewSubcategory(c, data)
}
