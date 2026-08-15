package messaging

import (
	"context"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
)

// finishMovementNegativeConfirmFlow es el puente al finish que ahora vive en
// flow (FinishMovementNegativeConfirm). Los tests del borde lo llaman por este
// nombre; el puente se borra al cerrar la costura.
func (c *controller) finishMovementNegativeConfirmFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	flow.FinishMovementNegativeConfirm(ctx, c, b, chatID, data)
}
