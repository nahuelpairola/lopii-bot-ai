package messaging

import (
	"context"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/settings"
	"lopiibot.com/internal/subcategory"
)

func (c *controller) finishCategoryManagePickFlow(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	flow.FinishCategoryManagePickFlow(ctx, c, chat, data)
}

func (c *controller) finishCategoryManageTargetFlow(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	flow.FinishCategoryManageTargetFlow(ctx, c, chat, data)
}

func (c *controller) proceedToCategoryTarget(ctx context.Context, chat messenger.Chat, data conversation.Data) error {
	return flow.ProceedToCategoryTarget(ctx, c, chat, data)
}

func (c *controller) suggestMergeTarget(ctx context.Context, userID, sourceID uint64, data conversation.Data) *subcategory.Subcategory {
	return settings.SuggestMergeTarget(ctx, c, userID, sourceID, data)
}
