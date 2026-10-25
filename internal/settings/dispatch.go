package settings

import (
	"context"
	"log/slog"

	"lopiibot.com/internal/agent"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
)

func (w *Wizards) Dispatch(ctx context.Context, chat messenger.Chat, userID uint64, text, area string) error {
	switch area {
	case agent.SettingsAreaAccount:
		return w.StartAccountManage(ctx, chat, userID, text)
	case agent.SettingsAreaCategory:
		return w.StartSubcategorySetup(ctx, chat, userID, text)
	case agent.SettingsAreaCategoryManage:
		return w.StartCategoryManage(ctx, chat, userID)
	case agent.SettingsAreaReminder:
		return w.StartReminderSetup(ctx, chat, userID)
	default:
		slog.WarnContext(ctx, "manage_settings con área desconocida", "user_id", userID, "area", area)
		flow.SendText(ctx, chat, agent.MsgAskRewrite)
		return nil
	}
}
