package traceutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

type contextKey string

const traceKey contextKey = "trace_id"

func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceKey, traceID)
}

func TraceIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(traceKey).(string)
	return v
}

func EnsureTraceID(ctx context.Context) (context.Context, string) {
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		return ctx, traceID
	}
	traceID := Generate()
	return WithTraceID(ctx, traceID), traceID
}

func Generate() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "trace-fallback"
	}
	return hex.EncodeToString(raw[:])
}
