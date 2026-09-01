package flow

import (
	"context"
	"strconv"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/reminder"
)

func FinishReminderSetup(ctx context.Context, r runner, chat messenger.Chat, data conversation.Data) {
	userID := data.UserID()

	if conversation.Flag(data, conversation.KeyCancelled) {
		r.SendText(ctx, chat, MsgReminderCancelled)
		return
	}

	switch conversation.StringOrEmpty(data[ReminderActionKey]) {
	case ReminderActionOff:
		if err := r.DisableReminder(userID); err != nil {
			r.SendText(ctx, chat, MsgCouldNotSave("tu recordatorio"))
			return
		}
		r.SendText(ctx, chat, MsgReminderDisabled)
		return

	case ReminderActionOffAll:
		if err := r.DisableReminder(userID); err != nil {
			r.SendText(ctx, chat, MsgCouldNotSave("tu recordatorio"))
			return
		}
		if err := r.SetWeeklySummary(userID, false); err != nil {
			r.SendText(ctx, chat, MsgCouldNotSave("el resumen semanal"))
			return
		}
		r.SendText(ctx, chat, MsgReminderAllOff)
		return

	case ReminderActionSoftExit:
		r.SendText(ctx, chat, MsgReminderHubExit)
		return

	case ReminderActionWeeklyOnly:
		on := conversation.Flag(data, conversation.KeyWeeklySummary)
		if on && !conversation.Flag(data, KeyHubHasRow) {
			if err := r.UpsertReminder(&reminder.Reminder{
				UserID:               userID,
				Enabled:              false,
				WeeklySummaryEnabled: true,
			}); err != nil {
				r.SendText(ctx, chat, MsgCouldNotSave("el resumen semanal"))
				return
			}
			r.SendText(ctx, chat, MsgWeeklySummaryOn)
			return
		}
		if err := r.SetWeeklySummary(userID, on); err != nil {
			r.SendText(ctx, chat, MsgCouldNotSave("el resumen semanal"))
			return
		}
		if on {
			r.SendText(ctx, chat, MsgWeeklySummaryOn)
		} else {
			r.SendText(ctx, chat, MsgWeeklySummaryOff)
		}
		return
	}

	startMin, err := strconv.Atoi(conversation.StringOrEmpty(data[ReminderStartKey]))
	endMin, err2 := strconv.Atoi(conversation.StringOrEmpty(data[ReminderEndKey]))
	if err != nil || err2 != nil {
		startMin, endMin, err = ParseWindow(conversation.StringOrEmpty(data[ReminderCustomKey]))
		if err != nil {
			r.SendText(ctx, chat, MsgSomethingBroke)
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
		r.SendText(ctx, chat, MsgCouldNotSave("tu recordatorio"))
		return
	}
	r.SendText(ctx, chat, MsgReminderSet(startMin, endMin))
}
