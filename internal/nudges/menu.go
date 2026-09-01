package nudges

import (
	"context"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
)

const menuMaxOptions = 4

func eligibleQuestions(s Services, userID uint64, stats *nudgeStats) []nudgeDef {
	var out []nudgeDef
	for _, n := range nudges {
		if n.question == "" || n.recurring {
			continue
		}
		if n.when(s, userID, stats) {
			out = append(out, n)
		}
	}
	return out
}

func sendQuestionMenu(ctx context.Context, s Services, chat messenger.Chat, userID uint64) {
	opts := eligibleQuestions(s, userID, buildNudgeStats(s, userID))
	if len(opts) == 0 {
		s.SendText(ctx, chat, msgMenuNoData)
		return
	}
	if len(opts) > menuMaxOptions {
		opts = opts[:menuMaxOptions]
	}
	buttons := make([]conversation.Button, 0, len(opts))
	for _, n := range opts {
		buttons = append(buttons, conversation.Button{Label: n.question, Data: nudgeQueryPrefix + n.key})
	}
	s.SendPrompt(ctx, chat, conversation.Prompt{Text: msgMenuHeader, Buttons: buttons})
}
