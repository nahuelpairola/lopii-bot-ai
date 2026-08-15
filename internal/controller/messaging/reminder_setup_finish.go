package messaging

import (
	"context"
	"strconv"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/reminder"
)

// finishReminderSetup applies the completed reminder_setup flow: disable, set
// (preset or custom), or cancel — then sends a receipt. No confirm gate.
func (c *controller) finishReminderSetup(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	userID := data.UserID()

	if conversation.Flag(data, conversation.KeyCancelled) {
		c.sendText(ctx, b, chatID, msgReminderCancelled)
		return
	}

	switch conversation.StringOrEmpty(data[flow.ReminderActionKey]) {
	case flow.ReminderActionOff:
		if err := c.reminders.Disable(userID); err != nil {
			c.sendText(ctx, b, chatID, msgCouldNotSave("tu recordatorio"))
			return
		}
		c.sendText(ctx, b, chatID, msgReminderDisabled)
		return

	case flow.ReminderActionOffAll:
		if err := c.reminders.Disable(userID); err != nil {
			c.sendText(ctx, b, chatID, msgCouldNotSave("tu recordatorio"))
			return
		}
		if err := c.reminders.SetWeeklySummary(userID, false); err != nil {
			c.sendText(ctx, b, chatID, msgCouldNotSave("el resumen semanal"))
			return
		}
		c.sendText(ctx, b, chatID, msgReminderAllOff)
		return

	case flow.ReminderActionSoftExit:
		c.sendText(ctx, b, chatID, msgReminderHubExit)
		return

	case flow.ReminderActionWeeklyOnly:
		on := conversation.Flag(data, conversation.KeyWeeklySummary)
		if on && !conversation.Flag(data, flow.KeyHubHasRow) {
			// no row yet: SetWeeklySummary is UPDATE-only and would no-op.
			// Create a minimal weekly-only row (daily disabled).
			if err := c.reminders.Upsert(&reminder.Reminder{
				UserID:               userID,
				Enabled:              false,
				WeeklySummaryEnabled: true,
			}); err != nil {
				c.sendText(ctx, b, chatID, msgCouldNotSave("el resumen semanal"))
				return
			}
			c.sendText(ctx, b, chatID, msgWeeklySummaryOn)
			return
		}
		if err := c.reminders.SetWeeklySummary(userID, on); err != nil {
			c.sendText(ctx, b, chatID, msgCouldNotSave("el resumen semanal"))
			return
		}
		if on {
			c.sendText(ctx, b, chatID, msgWeeklySummaryOn)
		} else {
			c.sendText(ctx, b, chatID, msgWeeklySummaryOff)
		}
		return
	}

	// set: a preset stored start/end mins directly; the custom path stored raw
	// text validated by flow.ParseWindow, so re-parsing here cannot fail.
	startMin, err := strconv.Atoi(conversation.StringOrEmpty(data[flow.ReminderStartKey]))
	endMin, err2 := strconv.Atoi(conversation.StringOrEmpty(data[flow.ReminderEndKey]))
	if err != nil || err2 != nil {
		startMin, endMin, err = flow.ParseWindow(conversation.StringOrEmpty(data[flow.ReminderCustomKey]))
		if err != nil {
			c.sendText(ctx, b, chatID, msgSomethingBroke)
			return
		}
	}

	if err := c.reminders.Upsert(&reminder.Reminder{
		UserID:               userID,
		WindowStartMin:       startMin,
		WindowEndMin:         endMin,
		Enabled:              true,
		WeeklySummaryEnabled: conversation.Flag(data, conversation.KeyWeeklySummary),
	}); err != nil {
		c.sendText(ctx, b, chatID, msgCouldNotSave("tu recordatorio"))
		return
	}
	c.sendText(ctx, b, chatID, msgReminderSet(startMin, endMin))
}
