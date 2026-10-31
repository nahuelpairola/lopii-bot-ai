package flow

import (
	"context"
	"fmt"
	"log/slog"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
)

func StartFlow(ctx context.Context, engine *conversation.Engine, chat messenger.Chat, userID uint64, flowName string, seed conversation.Data, errCtx string) error {
	prompt, err := engine.StartWithData(userID, flowName, seed)
	if err != nil {
		SendText(ctx, chat, MsgSomethingBroke)
		return fmt.Errorf("%s: %w", errCtx, err)
	}
	SendPrompt(ctx, chat, prompt)
	return nil
}

func SendPrompt(ctx context.Context, chat messenger.Chat, prompt conversation.Prompt) {
	if err := chat.Send(ctx, prompt); err != nil {
		slog.ErrorContext(ctx, "send prompt failed", "err", err)
	}
}

func SendText(ctx context.Context, chat messenger.Chat, text string) {
	SendPrompt(ctx, chat, conversation.Prompt{Text: text})
}
