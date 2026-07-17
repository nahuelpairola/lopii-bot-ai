package messaging

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// handleWeeklySummaryOff powers the "🔕 Desactivar resumen" button on the weekly
// summary message: it turns off ONLY the weekly summary (not the daily reminder)
// and rewrites the message to a confirmation, clearing the button.
func (c *controller) handleWeeklySummaryOff(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	if cb == nil {
		return
	}
	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})

	telegramID := updateTelegramID(update)
	if telegramID == "" {
		return
	}
	u, err := c.users.FindByTelegramID(telegramID)
	if err != nil {
		slog.ErrorContext(ctx, "weekly off user lookup failed", "err", err)
		return
	}
	if err := c.reminders.SetWeeklySummary(u.ID, false); err != nil {
		slog.ErrorContext(ctx, "weekly off disable failed", "user_id", u.ID, "err", err)
		return
	}

	if cb.Message.Message == nil {
		return
	}
	b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      cb.Message.Message.Chat.ID,
		MessageID:   cb.Message.Message.ID,
		Text:        msgWeeklySummaryDisabled,
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}},
	})
}
