package messaging

import (
	"context"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/agent"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/pendingjob"
)

// pendingjobServices implements pendingjob.Services for *controller.
var _ pendingjob.Services = (*controller)(nil)

// UsersFindByID is already implemented in nudges_services.go (shared bridge).

func (c *controller) HandleFreeText(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) error {
	return c.handleFreeText(ctx, b, chatID, userID, text)
}

func (c *controller) ProceedToUpdateConfirm(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, message string, transactionID string, oldIDs []string, beforeRows []movement.MovementRow) error {
	return agent.ProceedToUpdateConfirm(ctx, c, b, chatID, userID, message, transactionID, oldIDs, beforeRows, agent.ChangeAsk{})
}

// SendText is already exported (flow runner bridge); no bridge needed.

// Traced bridges the unexported c.traced to the exported pendingjob.Services interface.
func (c *controller) Traced(ctx context.Context, updateType string, raw string, fn func(tctx context.Context) (*uint64, error)) {
	c.traced(ctx, updateType, raw, fn)
}
