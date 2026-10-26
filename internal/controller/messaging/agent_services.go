package messaging

import (
	"context"

	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/pendingjob"
)

func (c *controller) HandleGroqError(ctx context.Context, chat messenger.Chat, userID uint64, text string, err error) (bool, error) {
	return pendingjob.HandleGroqError(ctx, c, c.jobs, chat, userID, text, err)
}

func (c *controller) EnqueueUpdatePickIfRateLimited(ctx context.Context, chat messenger.Chat, userID uint64, message, transactionID string, oldIDs []string, beforeRows []movement.MovementRow, err error) bool {
	return pendingjob.EnqueueUpdatePick(ctx, c, c.jobs, chat, userID, message, transactionID, oldIDs, beforeRows, err)
}
