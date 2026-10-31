package settings

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
)

func (w *Wizards) StartReminderSetup(ctx context.Context, chat messenger.Chat, userID uint64) error {
	slog.InfoContext(ctx, "flow started", "flow", flow.ReminderSetupFlowName, "user_id", userID)
	seed := conversation.Data{}
	if rem, err := w.reminders.FindByUserID(userID); err == nil && rem != nil {
		conversation.SetFlag(seed, flow.KeyHubHasRow)
		if rem.Enabled {
			conversation.SetFlag(seed, flow.KeyHubDailyOn)
			seed[flow.ReminderStartKey] = strconv.Itoa(rem.WindowStartMin)
			seed[flow.ReminderEndKey] = strconv.Itoa(rem.WindowEndMin)
		}
		if rem.WeeklySummaryEnabled {
			conversation.SetFlag(seed, conversation.KeyWeeklySummary)
		}
	}
	prompt, err := w.engine.StartWithData(userID, flow.ReminderSetupFlowName, seed)
	if err != nil {
		flow.SendText(ctx, chat, flow.MsgSomethingBroke)
		return fmt.Errorf("start reminder_setup flow: %w", err)
	}
	flow.SendPrompt(ctx, chat, prompt)
	return nil
}
