package messaging

import (
	"context"
	"strconv"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
)

const (
	movementDeleteFlowName = "movement_delete"

	stepPickDeleteCandidate = "pick_delete_candidate"
	stepConfirmDelete       = "confirm_delete"
)

// NewMovementDeleteFlow is a single flow, unlike UPDATE's two-hop
// chain: once a candidate is known, deleting needs no further LLM
// call, so the picker step just skips straight to confirm (via Skip,
// Task 1) whenever the caller already seeded a resolved_index.
func NewMovementDeleteFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		stepPickDeleteCandidate: conversation.ChoiceStep{
			PromptText: msgPickDeleteCandidate,
			SkipIf: func(data conversation.Data) (string, bool) {
				if stringOrEmpty(data[keyResolvedIndex]) != "" {
					return stepConfirmDelete, true
				}
				return "", false
			},
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				labels := decodeStringSlice(data, keyCandidateLabels)
				opts := make([]conversation.ChoiceOption, 0, len(labels))
				for i, label := range labels {
					opts = append(opts, conversation.ChoiceOption{
						Label:    label,
						Value:    strconv.Itoa(i),
						NextStep: stepConfirmDelete,
					})
				}
				return opts
			},
			DeclaredNextSteps: []string{stepConfirmDelete},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := copyData(data)
				next[keyResolvedIndex] = value
				return next
			},
			InvalidChoiceMessage: msgInvalidChoice,
		},
		stepConfirmDelete: conversation.ChoiceStep{
			PromptText: msgConfirmDelete,
			Options: []conversation.ChoiceOption{
				{Label: "🗑️ Confirmar borrado", Value: optionConfirm, Finish: true},
				{Label: "❌ Cancelar", Value: "cancel", Finish: true},
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := copyData(data)
				next[keyConfirmed] = strconv.FormatBool(value == optionConfirm)
				return next
			},
			InvalidChoiceMessage: msgInvalidChoice,
		},
	}

	flow, err := conversation.NewFlow(movementDeleteFlowName, stepPickDeleteCandidate, steps)
	if err != nil {
		panic(err)
	}
	return flow
}

// finishMovementDeleteFlow applies (or discards) the delete depending
// on which button the user pressed.
func (c *controller) finishMovementDeleteFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if !flag(data, keyConfirmed) {
		c.resolveMetric(ctx, data.UserID(), outcomeDeleteCancelled)
		if b != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgDeleteCancelled})
		}
		return
	}

	idx, err := strconv.Atoi(stringOrEmpty(data[keyResolvedIndex]))
	candidates := decodeCandidateGroups(data)
	if err != nil || idx < 0 || idx >= len(candidates) {
		if b != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgSomethingBroke})
		}
		return
	}

	ids, err := parseUintSlice(candidates[idx].OldIDs)
	if err != nil {
		if b != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgSomethingBroke})
		}
		return
	}

	if err := c.movements.SoftDeleteByIDs(ids); err != nil {
		if b != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgCouldNotDelete("tu movimiento")})
		}
		return
	}

	c.resolveMetric(ctx, data.UserID(), outcomeDeleteConfirmed, ids...)
	if b != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgDeleteApplied})
	}
}
