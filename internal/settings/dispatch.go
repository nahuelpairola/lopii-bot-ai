package settings

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/agent"
	"lopiibot.com/internal/messages"
)

// Dispatch despacha al wizard del área que el loop eligió.
//
// El loop ya leyó el mensaje, así que elegir el área no cuesta una llamada
// extra. Cada especialista toma el texto crudo, que es la razón por la que
// manage_settings no lleva un campo de texto: no hay nada que el modelo tenga
// que reescribir.
//
// Las áreas son el enum del schema y viven en agent (agent.SettingsArea*), el
// dueño del tool manage_settings que el modelo elige.
func Dispatch(ctx context.Context, s Services, b *bot.Bot, chatID int64, userID uint64, text, area string) error {
	switch area {
	case agent.SettingsAreaAccount:
		return StartAccountManage(ctx, s, b, chatID, userID, text)
	case agent.SettingsAreaCategory:
		return StartSubcategorySetup(ctx, s, b, chatID, userID, text)
	case agent.SettingsAreaCategoryManage:
		// Sacar o fusionar una categoría propia es OTRO wizard, y hasta el
		// 2026-08-12 era inalcanzable: las dos cosas compartían área y el área
		// entera iba al wizard de ALTA. "Elimina subcategorias" abría "crear
		// categoría nueva". Al morir el router, category_manage_pick se quedó sin
		// ningún camino que lo abriera.
		return StartCategoryManage(ctx, s, b, chatID, userID)
	case agent.SettingsAreaReminder:
		return StartReminderSetup(ctx, s, b, chatID, userID)
	default:
		slog.WarnContext(ctx, "manage_settings con área desconocida", "user_id", userID, "area", area)
		s.SendText(ctx, b, chatID, messages.MsgAskRewrite)
		return nil
	}
}
