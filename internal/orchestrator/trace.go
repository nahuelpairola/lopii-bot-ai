package orchestrator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

type ctxKey int

const traceIDKey ctxKey = iota

// NewTraceID genera un id de correlación aleatorio (16 bytes → 32 hex).
// crypto/rand, sin dependencia externa (no google/uuid).
func NewTraceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand no falla en la práctica; id vacío es aceptable si lo hiciera
	return hex.EncodeToString(b)
}

func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceIDKey, id)
}

// TraceID devuelve el trace_id del ctx, o "" si no hay.
func TraceID(ctx context.Context) string {
	v, _ := ctx.Value(traceIDKey).(string)
	return v
}
