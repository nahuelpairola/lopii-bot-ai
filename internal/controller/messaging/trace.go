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
	traceID := trace.NewID()
	ctx = trace.WithID(ctx, traceID)
	start := time.Now()

	slog.InfoContext(ctx, "update received", "update_type", updateType(update))
	slog.DebugContext(ctx, "raw update", "text", rawText(update))

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
	if e := c.traces.InsertRequestTrace(traceID, userID, updateType(update), start, int(time.Since(start).Milliseconds()), errMsg); e != nil {
		slog.ErrorContext(ctx, "request_trace insert failed", "err", e)
	}
}

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
