package messaging

import (
	"context"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
)

func (c *controller) finishCategoryManagePickFlow(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	c.finisher().FinishCategoryManagePickFlow(ctx, chat, data)
}

func (c *controller) finishCategoryManageTargetFlow(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	c.finisher().FinishCategoryManageTargetFlow(ctx, chat, data)
}

func (c *controller) proceedToCategoryTarget(ctx context.Context, chat messenger.Chat, data conversation.Data) error {
	return c.finisher().ProceedToCategoryTarget(ctx, chat, data)
}
