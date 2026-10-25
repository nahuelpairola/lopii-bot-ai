package messaging

import (
	"context"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
)

func (c *controller) finishSubcategorySetupFlow(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	c.finisher().FinishSubcategorySetup(ctx, chat, data)
}

func (c *controller) insertNewSubcategory(data conversation.Data) error {
	return c.finisher().InsertNewSubcategory(data)
}
