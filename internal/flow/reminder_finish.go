package flow

import (
	"context"
	"strconv"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/reminder"
)

func (f *Finisher) FinishReminderSetup(ctx context.Context, chat messenger.Chat, data conversation.Data) {
	userID := data.UserID()

	if conversation.Flag(data, conversation.KeyCancelled) {
		SendText(ctx, chat, MsgReminderCancelled)
		return
	}

	switch conversation.StringOrEmpty(data[ReminderActionKey]) {
	case ReminderActionOff:
		if err := f.reminders.Disable(userID); err != nil {
			SendText(ctx, chat, MsgCouldNotSave("tu recordatorio"))
			return
		}
		SendText(ctx, chat, MsgReminderDisabled)
		return

	case ReminderActionOffAll:
		if err := f.reminders.Disable(userID); err != nil {
			SendText(ctx, chat, MsgCouldNotSave("tu recordatorio"))
			return
		}
		if err := f.reminders.SetWeeklySummary(userID, false); err != nil {
			SendText(ctx, chat, MsgCouldNotSave("el resumen semanal"))
			return
		}
		SendText(ctx, chat, MsgReminderAllOff)
		return

	case ReminderActionSoftExit:
		SendText(ctx, chat, MsgReminderHubExit)
		return

	case ReminderActionWeeklyOnly:
		on := conversation.Flag(data, conversation.KeyWeeklySummary)
		if on && !conversation.Flag(data, KeyHubHasRow) {
			if err := f.reminders.Upsert(&reminder.Reminder{
				UserID:               userID,
				Enabled:              false,
				WeeklySummaryEnabled: true,
			}); err != nil {
				SendText(ctx, chat, MsgCouldNotSave("el resumen semanal"))
				return
			}
			SendText(ctx, chat, MsgWeeklySummaryOn)
			return
		}
		if err := f.reminders.SetWeeklySummary(userID, on); err != nil {
			SendText(ctx, chat, MsgCouldNotSave("el resumen semanal"))
			return
		}
		if on {
			SendText(ctx, chat, MsgWeeklySummaryOn)
		} else {
			SendText(ctx, chat, MsgWeeklySummaryOff)
		}
		return
	}

	startMin, err := strconv.Atoi(conversation.StringOrEmpty(data[ReminderStartKey]))
	endMin, err2 := strconv.Atoi(conversation.StringOrEmpty(data[ReminderEndKey]))
	if err != nil || err2 != nil {
		startMin, endMin, err = ParseWindow(conversation.StringOrEmpty(data[ReminderCustomKey]))
		if err != nil {
			SendText(ctx, chat, MsgSomethingBroke)
			return
		}
	}

	if err := f.reminders.Upsert(&reminder.Reminder{
		UserID:               userID,
		WindowStartMin:       startMin,
		WindowEndMin:         endMin,
		Enabled:              true,
		WeeklySummaryEnabled: conversation.Flag(data, conversation.KeyWeeklySummary),
	}); err != nil {
		SendText(ctx, chat, MsgCouldNotSave("tu recordatorio"))
		return
	}
	SendText(ctx, chat, MsgReminderSet(startMin, endMin))
}
