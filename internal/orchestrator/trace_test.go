package orchestrator

import (
	"context"
	"testing"
)

func TestTraceIDRoundtrip(t *testing.T) {
	ctx := WithTraceID(context.Background(), "abc123")
	if got := TraceID(ctx); got != "abc123" {
		t.Fatalf("got %q", got)
	}
	// ctx sin trace → "" (no panic)
	if got := TraceID(context.Background()); got != "" {
		t.Fatalf("missing trace must be empty, got %q", got)
	}
}

func TestNewTraceIDUnique(t *testing.T) {
	a, b := NewTraceID(), NewTraceID()
	if a == "" || a == b {
		t.Fatalf("ids must be non-empty and unique: %q %q", a, b)
	}
	if len(a) != 32 { // 16 bytes hex
		t.Fatalf("want 32 hex chars, got %d", len(a))
	}
}
