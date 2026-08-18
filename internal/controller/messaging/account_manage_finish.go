package messaging

import (
	"context"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
)

// Los finishes de ACCOUNT_MANAGE viven en flow/account_finish.go. Estos
// delegadores conservan los nombres de borde mientras los tests y
// handleFlowFinished los usen.
func (c *controller) finishAccountManageFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	flow.FinishAccountManage(ctx, c, b, chatID, data)
}

func (c *controller) finishAccountAdjust(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	flow.FinishAccountAdjust(ctx, c, b, chatID, data)
}

func (c *controller) finishAccountDefault(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	flow.FinishAccountDefault(ctx, c, b, chatID, data)
}

func (c *controller) finishAccountMoveOffer(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	flow.FinishAccountMoveOffer(ctx, c, b, chatID, data)
}
