package messaging

import (
	"context"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
)

// finishAccountCreateFlow delega en flow.FinishAccountCreate (account_finish.go).
// Conserva el nombre de borde mientras los tests y handleFlowFinished lo usen.
func (c *controller) finishAccountCreateFlow(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	flow.FinishAccountCreate(ctx, c, chat, data)
}
