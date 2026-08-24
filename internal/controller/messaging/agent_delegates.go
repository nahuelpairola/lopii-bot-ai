package messaging

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/pendingjob"
	"lopiibot.com/internal/settings"
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
	if handled, oerr := pendingjob.HandleGroqError(ctx, pendingjobBridge{c}, c.jobs, b, chatID, userID, text, qErr); handled {
		return oerr
	}
	if qErr != nil {
		slog.ErrorContext(ctx, "query failed", "user_id", userID, "err", qErr)
	}
	c.sendText(ctx, b, chatID, msgQueryFailed)
	c.resolveMetric(ctx, userID, outcomeQueryFailed)
	return qErr
}

// finishManageSettings entrega al cluster de wizards de configuración. El
// despacho por área vive en settings.Dispatch (internal/settings).
func (c *controller) finishManageSettings(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text, area string) error {
	return settings.Dispatch(ctx, settingsBridge{c}, b, chatID, userID, text, area)
}
