package messaging

import (
	"context"
	"strconv"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
)

// finishMovementDeleteFlow applies (or discards) the delete depending
// on which button the user pressed.
func (c *controller) finishMovementDeleteFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if !conversation.Flag(data, conversation.KeyConfirmed) {
		c.resolveMetric(ctx, data.UserID(), outcomeDeleteCancelled)
		if b != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgDeleteCancelled})
		}
		return
	}

	idx, err := strconv.Atoi(conversation.StringOrEmpty(data[conversation.KeyResolvedIndex]))
	candidates := flow.DecodeCandidateGroups(data)
	if err != nil || idx < 0 || idx >= len(candidates) {
		if b != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgSomethingBroke})
		}
		return
	}

	ids, err := parseUintSlice(candidates[idx].OldIDs)
	if err != nil {
		if b != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgSomethingBroke})
		}
		return
	}

	if err := c.movements.SoftDeleteByIDs(ids); err != nil {
		if b != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgCouldNotDelete("tu movimiento")})
		}
		return
	}

	c.resolveMetric(ctx, data.UserID(), outcomeDeleteConfirmed, ids...)
	if b != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgDeleteApplied})
	}
}
