package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"lopiibot.com/internal/trace"
)

func TestTraceHandlerStampsTraceID(t *testing.T) {
	var buf bytes.Buffer
	h := traceHandler{slog.NewJSONHandler(&buf, nil)}
	logger := slog.New(h)

	ctx := trace.WithID(context.Background(), "abc123")
	logger.InfoContext(ctx, "hello")

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("log line not JSON: %v", err)
	}
	if rec["trace_id"] != "abc123" {
		t.Errorf("trace_id = %v, want abc123", rec["trace_id"])
	}

	buf.Reset()
	logger.InfoContext(context.Background(), "no-trace")
	if strings.Contains(buf.String(), "trace_id") {
		t.Errorf("unexpected trace_id in log without ctx id: %s", buf.String())
	}
}
