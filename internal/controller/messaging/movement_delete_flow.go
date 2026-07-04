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
				if stringOrEmpty(data["resolved_index"]) != "" {
					return stepConfirmDelete, true
				}
				return "", false
			},
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				labels := decodeStringSlice(data, "candidate_labels")
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
				next["resolved_index"] = value
				return next
			},
			InvalidChoiceMessage: msgGenericFlowError,
		},
		stepConfirmDelete: conversation.ChoiceStep{
			PromptText: msgConfirmDelete,
			Options: []conversation.ChoiceOption{
				{Label: "🗑️ Confirmar borrado", Value: "confirm", Finish: true},
				{Label: "❌ Cancelar", Value: "cancel", Finish: true},
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := copyData(data)
				next["confirmed"] = strconv.FormatBool(value == "confirm")
				return next
			},
			InvalidChoiceMessage: msgGenericFlowError,
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
	if stringOrEmpty(data["confirmed"]) != "true" {
		if b != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgDeleteCancelled})
		}
		return
	}

	idx, err := strconv.Atoi(stringOrEmpty(data["resolved_index"]))
	candidates := decodeCandidateGroups(data)
	if err != nil || idx < 0 || idx >= len(candidates) {
		if b != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgGenericFlowError})
		}
		return
	}

	ids, err := parseUintSlice(candidates[idx].OldIDs)
	if err != nil {
		if b != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgGenericFlowError})
		}
		return
	}

	if err := c.movements.SoftDeleteByIDs(ids); err != nil {
		if b != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgGenericFlowError})
		}
		return
	}

	if b != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgDeleteApplied})
	}
}
