package messaging

import (
	"context"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
)

const (
	movementNegativeConfirmFlowName = "movement_negative_confirm"
	stepNegativeConfirm             = "negative_confirm"
)

// NewMovementNegativeConfirmFlow is the insufficient-funds gate: a well-formed
// CREATE that would drive an account negative stops here instead of inserting.
// Three exits — register as-is, rewrite, or "something's missing" (abort with a
// hint). Seeded (via StartWithData) with the pending movement rows so
// "Registrar igual" can re-insert them with the balance check skipped.
func NewMovementNegativeConfirmFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		stepNegativeConfirm: conversation.ChoiceStep{
			PromptText: func(data conversation.Data) string { return stringOrEmpty(data["_gate_prompt"]) },
			Options: []conversation.ChoiceOption{
				{Label: "✅ Registrar igual", Value: "register", Finish: true},
				{Label: "✍️ Reescribir", Value: "rewrite", Finish: true},
				{Label: "➕ Falta registrar algo", Value: "missing", Finish: true},
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := copyData(data)
				next["_gate_choice"] = value
				return next
			},
			InvalidChoiceMessage: msgGenericFlowError,
		},
	}
	flow, err := conversation.NewFlow(movementNegativeConfirmFlowName, stepNegativeConfirm, steps)
	if err != nil {
		panic(err)
	}
	return flow
}

// finishMovementNegativeConfirmFlow applies the user's choice.
func (c *controller) finishMovementNegativeConfirmFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	switch stringOrEmpty(data["_gate_choice"]) {
	case "register":
		data["_skip_balance_check"] = "true"
		inserted, err := c.resolveAndInsertMovements(data)
		if err != nil {
			c.sendText(ctx, b, chatID, createErrorCopy(err))
			return
		}
		c.resolveMetric(data.UserID(), outcomeCreateInserted)
		c.sendText(ctx, b, chatID, msgConfirmMovements(inserted))
	case "missing":
		c.sendText(ctx, b, chatID, msgLogMissingFirst)
	default: // rewrite / anything else: drop it, the user re-sends
		c.sendText(ctx, b, chatID, msgOnboardingNotUnderstood)
	}
}
