package messaging

import (
	"context"
	"log/slog"
	"time"

	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/trace"
)

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
		return
	}
	if e := c.traces.InsertRequestTrace(traceID, userID, kind, start, int(time.Since(start).Milliseconds()), errMsg); e != nil {
		slog.ErrorContext(ctx, "request_trace insert failed", "err", e)
	}
}

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
