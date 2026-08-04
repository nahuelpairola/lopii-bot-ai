package messaging

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-telegram/bot/models"
	"lopiibot.com/internal/trace"
)

// withTrace envuelve un entrypoint: genera trace_id, lo mete en ctx, mide
// received→done y graba el spine. fn devuelve el userID resuelto (nil si no) y
// el error top-level (para request_traces.error). Fire-and-forget en el grabado.
func (c *controller) withTrace(ctx context.Context, update *models.Update, fn func(ctx context.Context) (*uint64, error)) {
	c.traced(ctx, updateType(update), rawText(update), fn)
}

// traced es el cuerpo de withTrace sin el *models.Update.
//
// Existe porque hay una segunda entrada al sistema que NO es un update de
// Telegram: el drenaje de pending_llm_jobs (job_drain.go), un ticker cuyo ctx
// es el del server. Sin esto, todo lo que nace de un mensaje que pasó por un
// 429 escribe llm_calls e intent_events con trace_id vacío — o sea que el
// mensaje se cae de las tres capas de trazabilidad justo cuando más interesa
// mirarlo, que es cuando algo salió mal.
//
// kind va a request_traces.update_type; raw es SOLO para el log de Debug (ver
// rawText: el texto del usuario no puede llegar a un log de tercero).
func (c *controller) traced(ctx context.Context, kind, raw string, fn func(ctx context.Context) (*uint64, error)) {
	traceID := trace.NewID()
	ctx = trace.WithID(ctx, traceID)
	start := time.Now()

	slog.InfoContext(ctx, "update received", "update_type", kind)
	slog.DebugContext(ctx, "raw update", "text", raw)

	userID, err := fn(ctx)

	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	slog.InfoContext(ctx, "update done",
		"latency_ms", time.Since(start).Milliseconds(),
		"err", errMsg,
	)

	if c.traces == nil {
		return // tests que construyen el controller sin traces
	}
	if e := c.traces.InsertRequestTrace(traceID, userID, kind, start, int(time.Since(start).Milliseconds()), errMsg); e != nil {
		slog.ErrorContext(ctx, "request_trace insert failed", "err", e)
	}
}

// updateTypeReplay es el update_type de un job drenado. Se distingue de text /
// callback / command a propósito: sin eso, un replay se lee en request_traces
// como si el usuario hubiera escrito de nuevo, y la latencia de la cola queda
// mezclada con la de los mensajes reales.
const updateTypeReplay = "replay"

// updateType clasifica el update para la columna update_type.
func updateType(u *models.Update) string {
	switch {
	case u.CallbackQuery != nil:
		return "callback"
	case u.Message != nil && len(u.Message.Text) > 0 && u.Message.Text[0] == '/':
		return "command"
	default:
		return "text"
	}
}

// rawText pulls the user's message text for Debug-level logging only. Never
// call this from an Info/Warn/Error site — raw text must not reach a
// third-party log (see the spec's privacy invariant).
func rawText(u *models.Update) string {
	switch {
	case u.CallbackQuery != nil:
		return u.CallbackQuery.Data
	case u.Message != nil:
		return u.Message.Text
	default:
		return ""
	}
}
