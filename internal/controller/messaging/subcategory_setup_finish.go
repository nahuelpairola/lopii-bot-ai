package messaging

import (
	"context"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
)

// finishSubcategorySetupFlow delega en flow.FinishSubcategorySetup
// (category_finish.go). Conserva los nombres de borde mientras los tests y
// handleFlowFinished lo usen.
func (c *controller) finishSubcategorySetupFlow(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	flow.FinishSubcategorySetup(ctx, c, chat, data)
}

func (c *controller) insertNewSubcategory(data conversation.Data) error {
	return flow.InsertNewSubcategory(c, data)
}
