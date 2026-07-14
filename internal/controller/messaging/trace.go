package messaging

import (
	"context"
	"log"
	"time"

	"github.com/go-telegram/bot/models"
	"lopiibot.com/internal/orchestrator"
)

// withTrace envuelve un entrypoint: genera trace_id, lo mete en ctx, mide
// received→done y graba el spine. fn devuelve el userID resuelto (nil si no) y
// el error top-level (para request_traces.error). Fire-and-forget en el grabado.
func (c *controller) withTrace(ctx context.Context, update *models.Update, fn func(ctx context.Context) (*uint64, error)) {
	traceID := orchestrator.NewTraceID()
	ctx = orchestrator.WithTraceID(ctx, traceID)
	start := time.Now()

	userID, err := fn(ctx)

	if c.traces == nil {
		return // tests que construyen el controller sin traces
	}
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	if e := c.traces.InsertRequestTrace(traceID, userID, updateType(update), start, int(time.Since(start).Milliseconds()), errMsg); e != nil {
		log.Printf("metric: insert request_trace: %v", e)
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
