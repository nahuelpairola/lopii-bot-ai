package trace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

type ctxKey int

const idKey ctxKey = iota

// NewID genera un id de correlación aleatorio (16 bytes → 32 hex).
// crypto/rand, sin dependencia externa (no google/uuid).
func NewID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand no falla en la práctica; id vacío es aceptable si lo hiciera
	return hex.EncodeToString(b)
}

// WithID mete el trace_id en el ctx. Transversal a propósito: lo produce el
// borde HTTP/Telegram y lo consumen orchestrator (llm_calls) y logging.
func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, idKey, id)
}

// ID devuelve el trace_id del ctx, o "" si no hay.
func ID(ctx context.Context) string {
	v, _ := ctx.Value(idKey).(string)
	return v
}
