package messaging

import (
	"context"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
)

// finishMovementDeleteFlow es el puente al finish que ahora vive en flow
// (FinishMovementDelete). Los tests del borde lo llaman por este nombre; el
// puente se borra al cerrar la costura.
func (c *controller) finishMovementDeleteFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	flow.FinishMovementDelete(ctx, c, b, chatID, data)
}
