package flow

import (
	"context"
	"strconv"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/reminder"
)

// FinishReminderSetup applies the completed reminder_setup flow: disable, set
// (preset or custom), or cancel — then sends a receipt. No confirm gate.
func FinishReminderSetup(ctx context.Context, r runner, b *bot.Bot, chatID int64, data conversation.Data) {
	userID := data.UserID()

	if conversation.Flag(data, conversation.KeyCancelled) {
		r.SendText(ctx, b, chatID, MsgReminderCancelled)
		return
	}

	switch conversation.StringOrEmpty(data[ReminderActionKey]) {
	case ReminderActionOff:
		if err := r.DisableReminder(userID); err != nil {
			r.SendText(ctx, b, chatID, MsgCouldNotSave("tu recordatorio"))
			return
		}
		r.SendText(ctx, b, chatID, MsgReminderDisabled)
		return

	case ReminderActionOffAll:
		if err := r.DisableReminder(userID); err != nil {
			r.SendText(ctx, b, chatID, MsgCouldNotSave("tu recordatorio"))
			return
		}
		if err := r.SetWeeklySummary(userID, false); err != nil {
			r.SendText(ctx, b, chatID, MsgCouldNotSave("el resumen semanal"))
			return
		}
		r.SendText(ctx, b, chatID, MsgReminderAllOff)
		return

	case ReminderActionSoftExit:
		r.SendText(ctx, b, chatID, MsgReminderHubExit)
		return

	case ReminderActionWeeklyOnly:
		on := conversation.Flag(data, conversation.KeyWeeklySummary)
		if on && !conversation.Flag(data, KeyHubHasRow) {
			// no row yet: SetWeeklySummary is UPDATE-only and would no-op.
			// Create a minimal weekly-only row (daily disabled).
			if err := r.UpsertReminder(&reminder.Reminder{
				UserID:               userID,
				Enabled:              false,
				WeeklySummaryEnabled: true,
			}); err != nil {
				r.SendText(ctx, b, chatID, MsgCouldNotSave("el resumen semanal"))
				return
			}
			r.SendText(ctx, b, chatID, MsgWeeklySummaryOn)
			return
		}
		if err := r.SetWeeklySummary(userID, on); err != nil {
			r.SendText(ctx, b, chatID, MsgCouldNotSave("el resumen semanal"))
			return
		}
		if on {
			r.SendText(ctx, b, chatID, MsgWeeklySummaryOn)
		} else {
			r.SendText(ctx, b, chatID, MsgWeeklySummaryOff)
		}
		return
	}

	// set: a preset stored start/end mins directly; the custom path stored raw
	// text validated by ParseWindow, so re-parsing here cannot fail.
	startMin, err := strconv.Atoi(conversation.StringOrEmpty(data[ReminderStartKey]))
	endMin, err2 := strconv.Atoi(conversation.StringOrEmpty(data[ReminderEndKey]))
	if err != nil || err2 != nil {
		startMin, endMin, err = ParseWindow(conversation.StringOrEmpty(data[ReminderCustomKey]))
		if err != nil {
			r.SendText(ctx, b, chatID, MsgSomethingBroke)
			return
		}
	}

	if err := r.UpsertReminder(&reminder.Reminder{
		UserID:               userID,
		WindowStartMin:       startMin,
		WindowEndMin:         endMin,
		Enabled:              true,
		WeeklySummaryEnabled: conversation.Flag(data, conversation.KeyWeeklySummary),
	}); err != nil {
		r.SendText(ctx, b, chatID, MsgCouldNotSave("tu recordatorio"))
		return
	}
	r.SendText(ctx, b, chatID, MsgReminderSet(startMin, endMin))
}
