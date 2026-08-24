package messaging

import (
	"context"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
)

// finishReminderSetup delega en flow.FinishReminderSetup (reminder_finish.go).
// Conserva el nombre de borde mientras los tests y handleFlowFinished lo usen.
func (c *controller) finishReminderSetup(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	flow.FinishReminderSetup(ctx, c, chat, data)
}
