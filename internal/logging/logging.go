// Package logging installs the process-wide slog logger. Its handler stamps the
// ctx's trace_id onto every record, so a log line can always be joined to its
// request_traces / llm_calls / intent_events rows.
package logging

import (
	"context"
	"log/slog"
	"os"

	"lopiibot.com/internal/trace"
)

// traceHandler stamps trace_id from ctx onto every record, so no call site can
// forget it. ponytail: embedding means WithAttrs/WithGroup return the inner
// handler and drop the stamp — harmless because nothing here uses slog.With
// (every call site passes attrs inline). If slog.With shows up, implement
// WithAttrs/WithGroup to re-wrap.
type traceHandler struct{ slog.Handler }

func (h traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := trace.ID(ctx); id != "" {
		r.AddAttrs(slog.String("trace_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

// Init installs the default logger. Unknown level/format fall back to the safe
// production pair rather than failing — a typo in a TOML must never stop the
// bot from booting.
func Init(level, format string) {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}
	var h slog.Handler
	if format == "text" {
		h = slog.NewTextHandler(os.Stdout, opts)
	} else {
		h = slog.NewJSONHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(traceHandler{h}))
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
