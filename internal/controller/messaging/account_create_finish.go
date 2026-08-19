package messaging

import (
	"context"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
)

// finishAccountCreateFlow delega en flow.FinishAccountCreate (account_finish.go).
// Conserva el nombre de borde mientras los tests y handleFlowFinished lo usen.
func (c *controller) finishAccountCreateFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	flow.FinishAccountCreate(ctx, c, b, chatID, data)
}
