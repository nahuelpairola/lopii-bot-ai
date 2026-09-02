package settings

import (
	"context"
	"log/slog"

	"lopiibot.com/internal/agent"
	"lopiibot.com/internal/messenger"
)

func Dispatch(ctx context.Context, s Services, chat messenger.Chat, userID uint64, text, area string) error {
	switch area {
	case agent.SettingsAreaAccount:
		return StartAccountManage(ctx, s, chat, userID, text)
	case agent.SettingsAreaCategory:
		return StartSubcategorySetup(ctx, s, chat, userID, text)
	case agent.SettingsAreaCategoryManage:
		return StartCategoryManage(ctx, s, chat, userID)
	case agent.SettingsAreaReminder:
		return StartReminderSetup(ctx, s, chat, userID)
	default:
		slog.WarnContext(ctx, "manage_settings con área desconocida", "user_id", userID, "area", area)
		s.SendText(ctx, chat, agent.MsgAskRewrite)
		return nil
	}
}
