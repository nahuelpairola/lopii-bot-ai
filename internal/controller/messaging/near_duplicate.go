package messaging

import (
	"context"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/movement"
)

// El gate de casi-duplicado vive en flow (near_duplicate.go /
// near_duplicate_offer.go). Estos delegadores conservan los nombres de borde
// mientras los tests y los callers (agent_executor.go, controller.go) los usen.
func (c *controller) maybeNearDuplicate(userID uint64, inserted []movement.Movement) []conversation.Button {
	return flow.MaybeNearDuplicate(c, userID, inserted)
}

func (c *controller) handleNearDuplicateChoice(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, data string) bool {
	return flow.HandleNearDuplicateChoice(ctx, c, b, chatID, userID, data)
}
