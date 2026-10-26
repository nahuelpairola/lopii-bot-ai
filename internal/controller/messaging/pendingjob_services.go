package messaging

import (
	"context"

	"lopiibot.com/internal/agent"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/pendingjob"
	"lopiibot.com/internal/user"
)

var _ pendingjob.Services = (*controller)(nil)

func (c *controller) HandleFreeText(ctx context.Context, chat messenger.Chat, userID uint64, text string) error {
	return c.loop().StartLoop(ctx, chat, userID, text)
}

func (c *controller) ProceedToUpdateConfirm(ctx context.Context, chat messenger.Chat, userID uint64, message string, transactionID string, oldIDs []string, beforeRows []movement.MovementRow) error {
	return c.loop().ProceedToUpdateConfirm(ctx, chat, userID, message, transactionID, oldIDs, beforeRows, agent.ChangeAsk{})
}

func (c *controller) Traced(ctx context.Context, updateType string, raw string, fn func(tctx context.Context) (*uint64, error)) {
	c.traced(ctx, updateType, raw, fn)
}

func (c *controller) UsersFindByID(userID uint64) (*user.User, error) {
	return c.users.FindByID(userID)
}
