package messaging

import (
	"context"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
)

const (
	movementConfirmFlowName = "movement_confirm_intent"
	stepConfirmIntent       = "confirm_intent"
)

// NewMovementConfirmFlow is the confirm gate shown when a CREATE
// classification is ambiguous — either the router flagged
// needs_confirmation, or resolveCandidates found a plausible duplicate
// (see startMovementCreate). It never guesses the user's real intent
// (no reroute to update/delete): only "reescribir" or "cancelar", both
// of which end the flow without touching the DB.
func NewMovementConfirmFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		stepConfirmIntent: conversation.ChoiceStep{
			PromptText: msgConfirmIntentUnclear,
			Options: []conversation.ChoiceOption{
				{Label: "✍️ Reescribir mensaje", Value: "rewrite", Finish: true},
				{Label: "🚫 Cancelar", Value: "cancel", Finish: true},
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := copyData(data)
				next["choice"] = value
				return next
			},
			InvalidChoiceMessage: msgGenericFlowError,
		},
	}

	flow, err := conversation.NewFlow(movementConfirmFlowName, stepConfirmIntent, steps)
	if err != nil {
		panic(err)
	}
	return flow
}

func (c *controller) finishMovementConfirmFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if stringOrEmpty(data["choice"]) == "rewrite" {
		c.resolveMetric(data.UserID(), outcomeCreateRewrite)
		c.sendText(ctx, b, chatID, msgAskRewrite)
		return
	}
	c.resolveMetric(data.UserID(), outcomeCreateCancelled)
	c.sendText(ctx, b, chatID, msgConfirmIntentCancelled)
}

// startMovementConfirm starts the confirm gate. Called from
// startMovementCreate whenever the router's needs_confirmation flag or
// the duplicate-check (resolveCandidates) signals an ambiguous CREATE.
func (c *controller) startMovementConfirm(ctx context.Context, b *bot.Bot, chatID int64, userID uint64) {
	prompt, err := c.engine.StartWithData(userID, movementConfirmFlowName, conversation.Data{})
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}
	if b != nil {
		c.sendPrompt(ctx, b, chatID, prompt)
	}
}
