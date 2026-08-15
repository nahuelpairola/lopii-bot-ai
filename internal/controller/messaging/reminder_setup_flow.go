package messaging

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
)

// startReminderSetup opens the Notificaciones hub: it reads the user's current
// reminder (if any) and seeds the panel state, so the root step can render both
// features' status and preserve the weekly flag when only the window changes.
func (c *controller) startReminderSetup(ctx context.Context, b *bot.Bot, chatID int64, userID uint64) error {
	slog.InfoContext(ctx, "flow started", "flow", flow.ReminderSetupFlowName, "user_id", userID)
	seed := conversation.Data{}
	if rem, err := c.reminders.FindByUserID(userID); err == nil && rem != nil {
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
	prompt, err := c.engine.StartWithData(userID, flow.ReminderSetupFlowName, seed)
	if err != nil {
		c.sendText(ctx, b, chatID, msgSomethingBroke)
		return fmt.Errorf("start reminder_setup flow: %w", err)
	}
	c.sendPrompt(ctx, b, chatID, prompt)
	return nil
}
