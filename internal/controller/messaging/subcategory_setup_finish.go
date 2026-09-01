package messaging

import (
	"context"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
)

func (c *controller) finishSubcategorySetupFlow(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	flow.FinishSubcategorySetup(ctx, c, chat, data)
}

func (c *controller) insertNewSubcategory(data conversation.Data) error {
	return flow.InsertNewSubcategory(c, data)
}
