package messaging

import (
	"context"

	"lopiibot.com/internal/agent"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/pendingjob"
)

var _ pendingjob.Replayer = (*controller)(nil)

func (c *controller) HandleFreeText(ctx context.Context, chat messenger.Chat, userID uint64, text string) error {
	return c.loop().StartLoop(ctx, chat, userID, text)
}

func (c *controller) ProceedToUpdateConfirm(ctx context.Context, chat messenger.Chat, userID uint64, message string, transactionID string, oldIDs []string, beforeRows []movement.MovementRow) error {
	return c.loop().ProceedToUpdateConfirm(ctx, chat, userID, message, transactionID, oldIDs, beforeRows, agent.ChangeAsk{})
}
