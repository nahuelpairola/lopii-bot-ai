package nudges

import (
	"context"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
)

const menuMaxOptions = 4

func (t *Tips) eligibleQuestions(userID uint64, stats *nudgeStats) []nudgeDef {
	var out []nudgeDef
	for _, n := range nudges {
		if n.question == "" || n.recurring {
			continue
		}
		if n.when(t, userID, stats) {
			out = append(out, n)
		}
	}
	return out
}

func (t *Tips) sendQuestionMenu(ctx context.Context, chat messenger.Chat, userID uint64) {
	opts := t.eligibleQuestions(userID, t.buildNudgeStats(userID))
	if len(opts) == 0 {
		flow.SendText(ctx, chat, msgMenuNoData)
		return
	}
	if len(opts) > menuMaxOptions {
		opts = opts[:menuMaxOptions]
	}
	buttons := make([]conversation.Button, 0, len(opts))
	for _, n := range opts {
		buttons = append(buttons, conversation.Button{Label: n.question, Data: nudgeQueryPrefix + n.key})
	}
	flow.SendPrompt(ctx, chat, conversation.Prompt{Text: msgMenuHeader, Buttons: buttons})
}
