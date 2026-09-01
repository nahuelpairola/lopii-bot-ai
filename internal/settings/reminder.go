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

func StartReminderSetup(ctx context.Context, s Services, chat messenger.Chat, userID uint64) error {
	slog.InfoContext(ctx, "flow started", "flow", flow.ReminderSetupFlowName, "user_id", userID)
	seed := conversation.Data{}
	if rem, err := s.RemindersFindByUserID(userID); err == nil && rem != nil {
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
	prompt, err := s.EngineStartWithData(userID, flow.ReminderSetupFlowName, seed)
	if err != nil {
		s.SendText(ctx, chat, flow.MsgSomethingBroke)
		return fmt.Errorf("start reminder_setup flow: %w", err)
	}
	s.SendPrompt(ctx, chat, prompt)
	return nil
}
