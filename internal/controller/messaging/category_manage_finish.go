package messaging

import (
	"context"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/settings"
	"lopiibot.com/internal/subcategory"
)

// Los finishes de CATEGORY_MANAGE viven en flow (category_finish.go). Estos
// delegadores conservan los nombres de borde mientras los tests y
// handleFlowFinished los usen.
func (c *controller) finishCategoryManagePickFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	flow.FinishCategoryManagePickFlow(ctx, c, b, chatID, data)
}

func (c *controller) finishCategoryManageTargetFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	flow.FinishCategoryManageTargetFlow(ctx, c, b, chatID, data)
}

func (c *controller) proceedToCategoryTarget(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) error {
	return flow.ProceedToCategoryTarget(ctx, c, b, chatID, data)
}

// suggestMergeTarget es el puente que flow alcanza via runner: la sugerencia de
// fusión toca el LLM y por eso vive en settings, que flow no importa.
func (c *controller) suggestMergeTarget(ctx context.Context, userID, sourceID uint64, data conversation.Data) *subcategory.Subcategory {
	return settings.SuggestMergeTarget(ctx, c, userID, sourceID, data)
}
