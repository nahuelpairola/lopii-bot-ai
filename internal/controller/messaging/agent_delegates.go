package messaging

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot"
)

// finishAnswerQuery entrega la consulta al loop de query.
//
// QUERY se queda en su propio loop y su propio modelo A PROPÓSITO: el techo de
// TPM de Groq es POR MODELO, así que una consulta no le come nada al bucket del
// loop unificado — que es justo el que está saturado. Meterla adentro movería
// ~800 tokens de schemas de lectura al bucket equivocado.
func (c *controller) finishAnswerQuery(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) error {
	answered, qErr := c.handleQuery(ctx, b, chatID, userID, text)
	if answered {
		c.resolveMetric(ctx, userID, outcomeQueryAnswered)
		return nil
	}
	// Un 429 encola y ackea: el intent_event sigue pendiente porque la historia
	// no terminó, la termina el drain. QUERY es el intent más seguro de
	// replayar: es read-only, no puede registrar la misma plata dos veces.
	if handled, oerr := c.handleGroqError(ctx, b, chatID, userID, text, qErr); handled {
		return oerr
	}
	if qErr != nil {
		slog.ErrorContext(ctx, "query failed", "user_id", userID, "err", qErr)
	}
	c.sendText(ctx, b, chatID, msgQueryFailed)
	c.resolveMetric(ctx, userID, outcomeQueryFailed)
	return qErr
}

// finishManageSettings despacha al wizard del área que el loop eligió.
//
// El loop ya leyó el mensaje, así que elegir el área no cuesta una llamada
// extra. Cada especialista de abajo toma el texto crudo, que es la razón por la
// que manage_settings no lleva un campo de texto: no hay nada que el modelo
// tenga que reescribir.
func (c *controller) finishManageSettings(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text, area string) error {
	switch area {
	case settingsAreaAccount:
		return c.startAccountManage(ctx, b, chatID, userID, text)
	case settingsAreaCategory:
		// Crear y administrar categorías comparten área: el modelo no puede saber
		// si lo que sigue es una alta o una fusión, y el wizard sí.
		return c.startSubcategorySetup(ctx, b, chatID, userID, text)
	case settingsAreaReminder:
		return c.startReminderSetup(ctx, b, chatID, userID)
	default:
		slog.WarnContext(ctx, "manage_settings con área desconocida", "user_id", userID, "area", area)
		c.sendText(ctx, b, chatID, msgAskRewrite)
		return nil
	}
}

// Las áreas de manage_settings. Son el enum del schema: si divergen, el modelo
// manda un área que el switch no conoce y el pedido muere en ask_rewrite.
const (
	settingsAreaAccount  = "cuenta"
	settingsAreaCategory = "categoria"
	settingsAreaReminder = "recordatorio"
)
