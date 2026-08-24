package messaging

import (
	"context"
	"log/slog"
	"time"

	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/trace"
)

// traced envuelve un entrypoint: genera trace_id, lo mete en ctx, mide
// received→done y graba el spine. fn devuelve el userID resuelto (nil si no) y
// el error top-level (para request_traces.error). Fire-and-forget en el grabado.
//
// Existe porque hay una segunda entrada al sistema que NO es un update de
// Telegram: el drenaje de pending_llm_jobs (job_drain.go), un ticker cuyo ctx
// es el del server. Sin esto, todo lo que nace de un mensaje que pasó por un
// 429 escribe llm_calls e intent_events con trace_id vacío — o sea que el
// mensaje se cae de las tres capas de trazabilidad justo cuando más interesa
// mirarlo, que es cuando algo salió mal.
//
// kind va a request_traces.update_type; raw es SOLO para el log de Debug (ver
// rawOf: el texto del usuario no puede llegar a un log de tercero).
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

// kindOf y rawOf reemplazan a updateType y rawText: derivaban del
// *models.Update y ahora derivan de Input, que es lo mismo sin el canal.
// raw es SOLO para el log de Debug — el texto del usuario no puede llegar a
// un log de tercero.
func kindOf(in messenger.Incoming) string {
	if in.Input.CallbackData != "" {
		return "callback"
	}
	return "text"
}

func rawOf(in messenger.Incoming) string {
	if in.Input.CallbackData != "" {
		return in.Input.CallbackData
	}
	return in.Input.Text
}
