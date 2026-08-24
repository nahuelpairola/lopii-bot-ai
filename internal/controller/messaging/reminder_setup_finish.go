package messaging

import (
	"context"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
)

// finishReminderSetup delega en flow.FinishReminderSetup (reminder_finish.go).
// Conserva el nombre de borde mientras los tests y handleFlowFinished lo usen.
func (c *controller) finishReminderSetup(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	flow.FinishReminderSetup(ctx, c, newEdgeChat(b, chatID), data)
}
