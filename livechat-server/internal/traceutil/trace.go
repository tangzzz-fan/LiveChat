package traceutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"google.golang.org/grpc/metadata"
)

type contextKey string

const (
	traceKey contextKey = "trace_id"
	spanKey  contextKey = "span_id"
)

func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceKey, traceID)
}

func TraceIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(traceKey).(string)
	return v
}

func WithSpanID(ctx context.Context, spanID string) context.Context {
	return context.WithValue(ctx, spanKey, spanID)
}

func SpanIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(spanKey).(string)
	return v
}

func EnsureTraceID(ctx context.Context) (context.Context, string) {
	traceID := TraceIDFromContext(ctx)
	if traceID != "" {
		return ctx, traceID
	}
	traceID = Generate()
	return WithTraceID(ctx, traceID), traceID
}

func Generate() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "trace-fallback"
	}
	return hex.EncodeToString(raw[:])
}

func GenerateSpanID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "span-fallback"
	}
	return hex.EncodeToString(raw[:])
}

// ── gRPC metadata propagation ─────────────────────

const (
	mdTraceID = "x-trace-id"
	mdSpanID  = "x-span-id"
)

// OutgoingGRPCContext returns a context with trace metadata for outgoing gRPC calls.
func OutgoingGRPCContext(ctx context.Context) context.Context {
	traceID := TraceIDFromContext(ctx)
	spanID := SpanIDFromContext(ctx)
	if spanID == "" {
		spanID = GenerateSpanID()
	}
	md := metadata.Pairs(mdTraceID, traceID, mdSpanID, spanID)
	return metadata.NewOutgoingContext(ctx, md)
}

// FromIncomingGRPC extracts trace info from incoming gRPC metadata.
func FromIncomingGRPC(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	if vals := md.Get(mdTraceID); len(vals) > 0 {
		ctx = WithTraceID(ctx, vals[0])
	}
	if vals := md.Get(mdSpanID); len(vals) > 0 {
		ctx = WithSpanID(ctx, vals[0])
	}
	return ctx
}
