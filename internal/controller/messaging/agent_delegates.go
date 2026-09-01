package messaging

import (
	"context"
	"log/slog"

	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/pendingjob"
	"lopiibot.com/internal/settings"
)

func (c *controller) finishAnswerQuery(ctx context.Context, chat messenger.Chat, userID uint64, text string) error {
	answered, qErr := c.handleQuery(ctx, chat, userID, text)
	if answered {
		c.resolveMetric(ctx, userID, outcomeQueryAnswered)
		return nil
	}
	if handled, oerr := pendingjob.HandleGroqError(ctx, c, c.jobs, chat, userID, text, qErr); handled {
		return oerr
	}
	if qErr != nil {
		slog.ErrorContext(ctx, "query failed", "user_id", userID, "err", qErr)
	}
	c.sendText(ctx, chat, msgQueryFailed)
	c.resolveMetric(ctx, userID, outcomeQueryFailed)
	return qErr
}

func (c *controller) finishManageSettings(ctx context.Context, chat messenger.Chat, userID uint64, text, area string) error {
	return settings.Dispatch(ctx, c, chat, userID, text, area)
}
