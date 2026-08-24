package messaging

import (
	"context"

	"lopiibot.com/internal/agent"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/pendingjob"
)

var _ pendingjob.Services = (*controller)(nil)

// pendingjobServices implements pendingjob.Services for *controller.
// *controller satisfies it directly now: every method here already speaks
// messenger.Chat, same as flow.runner and agentServices, so there is no
// bridge type left to wrap it (that was pendingjobBridge, deleted with
// Task 8 — see chat_bridge.go).

// UsersFindByID is already implemented in nudges_services.go (shared bridge).

// HandleFreeText no pasa por c.handleFreeText (free_text.go): ese helper
// existe para el borde del webhook, que todavía arma el chat a mano con
// (bot, chatID). Acá el chat ya viene resuelto — por chatResolver.ChatFor en
// el drenaje, nunca un edgeChat — así que se llama a agent.StartLoop
// directo, igual que hace handleFreeText por dentro.
func (c *controller) HandleFreeText(ctx context.Context, chat messenger.Chat, userID uint64, text string) error {
	return agent.StartLoop(ctx, c, chat, userID, text)
}

func (c *controller) ProceedToUpdateConfirm(ctx context.Context, chat messenger.Chat, userID uint64, message string, transactionID string, oldIDs []string, beforeRows []movement.MovementRow) error {
	return agent.ProceedToUpdateConfirm(ctx, c, chat, userID, message, transactionID, oldIDs, beforeRows, agent.ChangeAsk{})
}

// SendText already matches (ctx, messenger.Chat, string) via controller.go's
// implementation for flow.runner/agentServices — nothing to add here.

// Traced bridges the unexported c.traced to the exported pendingjob.Services interface.
func (c *controller) Traced(ctx context.Context, updateType string, raw string, fn func(tctx context.Context) (*uint64, error)) {
	c.traced(ctx, updateType, raw, fn)
}
