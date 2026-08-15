package messaging

import (
	"context"
	"errors"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/movement"
)

// finishMovementCreateFlow is the Telegram-facing wrapper around
// flow.ResolveAndInsertMovements — same split for testability as
// finishInitialBalanceFlow/insertInitialBalanceMovements.
func (c *controller) finishMovementCreateFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		c.resolveMetric(ctx, data.UserID(), outcomeCreateCancelled)
		if b != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgCreateCancelled})
		}
		return
	}

	inserted, err := flow.ResolveAndInsertMovements(c, data)
	if err != nil {
		// Saldo insuficiente no es una falla: es una pregunta. Los otros dos
		// caminos de CREATE (startMovementCreate y agentExecutor.record) ya la
		// hacían; éste no, y se comía el movimiento con un error genérico.
		var short *flow.InsufficientFunds
		if errors.As(err, &short) {
			gateSeed := conversation.CopyData(data)
			gateSeed[conversation.KeyGatePrompt] = msgInsufficientFunds(short.Shortfalls)
			if serr := c.startFlow(ctx, b, chatID, data.UserID(), flow.MovementNegativeConfirmFlowName, gateSeed, "create: start negative-confirm flow"); serr != nil {
				slog.ErrorContext(ctx, "negative-confirm flow failed to start", "user_id", data.UserID(), "error", serr)
			}
			return
		}
		slog.ErrorContext(ctx, "movement insert failed", "user_id", data.UserID(), "reason", guardReason(err))
		c.resolveMetric(ctx, data.UserID(), failureOutcomeFor(data))
		if b != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: createErrorCopy(err)})
		}
		return
	}
	c.resolveMetric(ctx, data.UserID(), writeOutcomeFor(data), collectMovementIDs(inserted)...)
	if b != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgConfirmMovements(inserted)})
		if name := conversation.StringOrEmpty(data[conversation.KeyFirstAccountName]); name != "" {
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgFirstAccountDefault(name, conversation.DecodeStringSlice(data, conversation.KeyFirstAccountCurrencies))})
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgInviteMoreAccounts})
			// R1/R2 just fired — mark correct_tip sent (not delivered) so the
			// post-message nudge hook doesn't stack a 3rd tip on this same turn.
			if c.nudges != nil {
				_ = c.nudges.MarkSent(data.UserID(), nudgeCorrectTip)
			}
		}
	}
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
