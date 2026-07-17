package trace

import (
	"context"
	"testing"
)

func TestIDRoundtrip(t *testing.T) {
	ctx := WithID(context.Background(), "abc123")
	if got := ID(ctx); got != "abc123" {
		t.Fatalf("ID(ctx) = %q, want %q", got, "abc123")
	}
	if got := ID(context.Background()); got != "" {
		t.Fatalf("ID(empty ctx) = %q, want empty", got)
	}
}

func TestNewIDUnique(t *testing.T) {
	a, b := NewID(), NewID()
	if a == b {
		t.Fatalf("NewID() returned the same id twice: %q", a)
	}
	if len(a) != 32 {
		t.Fatalf("NewID() length = %d, want 32", len(a))
	}
}
