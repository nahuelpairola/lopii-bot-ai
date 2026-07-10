package messaging

import (
	"context"
	"strconv"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/reminder"
)

// finishReminderSetup applies the completed reminder_setup flow: disable, set
// (preset or custom), or cancel — then sends a receipt. No confirm gate.
func (c *controller) finishReminderSetup(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	userID := data.UserID()

	if stringOrEmpty(data["cancelled"]) == "true" {
		c.sendText(ctx, b, chatID, msgReminderCancelled)
		return
	}

	if stringOrEmpty(data[reminderActionKey]) == reminderActionOff {
		if err := c.reminders.Disable(userID); err != nil {
			c.sendText(ctx, b, chatID, msgGenericFlowError)
			return
		}
		c.sendText(ctx, b, chatID, msgReminderDisabled)
		return
	}

	// set: a preset stored start/end mins directly; the custom path stored raw
	// text validated by parseWindow, so re-parsing here cannot fail.
	startMin, err := strconv.Atoi(stringOrEmpty(data[reminderStartKey]))
	endMin, err2 := strconv.Atoi(stringOrEmpty(data[reminderEndKey]))
	if err != nil || err2 != nil {
		startMin, endMin, err = parseWindow(stringOrEmpty(data[reminderCustomKey]))
		if err != nil {
			c.sendText(ctx, b, chatID, msgGenericFlowError)
			return
		}
	}

	if err := c.reminders.Upsert(&reminder.Reminder{
		UserID:         userID,
		WindowStartMin: startMin,
		WindowEndMin:   endMin,
		Enabled:        true,
	}); err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}
	c.sendText(ctx, b, chatID, msgReminderSet(startMin, endMin))
}
