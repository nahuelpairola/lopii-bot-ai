package messaging

import (
	"context"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
)

// finishMovementNegativeConfirmFlow applies the user's choice.
func (c *controller) finishMovementNegativeConfirmFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	switch conversation.StringOrEmpty(data["_gate_choice"]) {
	case "register":
		conversation.SetFlag(data, conversation.KeySkipBalanceCheck)
		inserted, err := c.resolveAndInsertMovements(data)
		if err != nil {
			c.sendText(ctx, b, chatID, createErrorCopy(err))
			return
		}
		c.resolveMetric(ctx, data.UserID(), writeOutcomeFor(data), collectMovementIDs(inserted)...)
		c.sendText(ctx, b, chatID, msgConfirmMovements(inserted))
	case "missing":
		c.sendText(ctx, b, chatID, msgLogMissingFirst)
	default: // rewrite / anything else: drop it, the user re-sends
		c.sendText(ctx, b, chatID, msgNotUnderstood)
	}
}
