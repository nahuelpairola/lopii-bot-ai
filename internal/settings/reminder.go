package settings

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
)

// StartReminderSetup opens the Notificaciones hub: it reads the user's current
// reminder (if any) and seeds the panel state, so the root step can render both
// features' status and preserve the weekly flag when only the window changes.
func StartReminderSetup(ctx context.Context, s Services, b *bot.Bot, chatID int64, userID uint64) error {
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
		s.SendText(ctx, b, chatID, flow.MsgSomethingBroke)
		return fmt.Errorf("start reminder_setup flow: %w", err)
	}
	s.SendPrompt(ctx, b, chatID, prompt)
	return nil
}
