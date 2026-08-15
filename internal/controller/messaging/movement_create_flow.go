package messaging

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/movement"
)

// finishMovementCreateFlow es el puente al finish que ahora vive en flow
// (FinishMovementCreate). Los tests del borde lo llaman por este nombre; el
// puente se borra al cerrar la costura.
func (c *controller) finishMovementCreateFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	flow.FinishMovementCreate(ctx, c, b, chatID, data)
}

// Los métodos de abajo son delgados puentes al pipeline que ahora vive en flow
// (movement_write.go). Los tests del borde los llaman por estos nombres.

func (c *controller) resolveAndInsertMovements(data conversation.Data) ([]movement.Movement, error) {
	return flow.ResolveAndInsertMovements(c, data)
}

func (c *controller) loadAccountIndex(userID uint64) (*flow.AccountIndex, error) {
	return flow.LoadAccountIndex(c, userID)
}

func (c *controller) createFirstAccount(data conversation.Data, rows []movement.MovementRow, idx *flow.AccountIndex) (bool, error) {
	return flow.CreateFirstAccount(c, data, rows, idx)
}

func firstAccountNetDelta(rows []movement.MovementRow, cur string) decimal.Decimal {
	return flow.FirstAccountNetDelta(rows, cur)
}

func fciRedemptionGain(c *controller, movements []movement.Movement) (movement.Movement, bool, error) {
	return flow.FciRedemptionGain(c, movements)
}
