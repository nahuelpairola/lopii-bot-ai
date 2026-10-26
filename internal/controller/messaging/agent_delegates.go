package messaging

import (
	"context"
	"log/slog"

	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/pendingjob"
)

func (c *controller) finishAnswerQuery(ctx context.Context, chat messenger.Chat, userID uint64, text string) error {
	answered, qErr := c.queries().Run(ctx, chat, userID, text)
	if answered {
		c.resolveMetric(ctx, userID, outcomeQueryAnswered)
		return nil
	}
	if handled, oerr := pendingjob.HandleGroqError(ctx, c.jobs, chat, userID, text, qErr); handled {
		return oerr
	}
	if qErr != nil {
		slog.ErrorContext(ctx, "query failed", "user_id", userID, "err", qErr)
	}
	flow.SendText(ctx, chat, msgQueryFailed)
	c.resolveMetric(ctx, userID, outcomeQueryFailed)
	return qErr
}
