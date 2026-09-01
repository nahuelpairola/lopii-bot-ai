package trace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

type ctxKey int

const idKey ctxKey = iota

func NewID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, idKey, id)
}

func ID(ctx context.Context) string {
	v, _ := ctx.Value(idKey).(string)
	return v
}
