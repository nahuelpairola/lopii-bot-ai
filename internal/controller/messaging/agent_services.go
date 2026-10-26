package messaging

import (
	"context"

	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/pendingjob"
)

func (c *controller) handleGroqError(ctx context.Context, chat messenger.Chat, userID uint64, text string, err error) (bool, error) {
	return pendingjob.HandleGroqError(ctx, c.jobs, chat, userID, text, err)
}

func (c *controller) enqueueUpdatePick(ctx context.Context, chat messenger.Chat, userID uint64, message, transactionID string, oldIDs []string, beforeRows []movement.MovementRow, err error) bool {
	return pendingjob.EnqueueUpdatePick(ctx, c.jobs, chat, userID, message, transactionID, oldIDs, beforeRows, err)
}
