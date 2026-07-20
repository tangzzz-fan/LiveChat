package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithLoggingGeneratesTraceID(t *testing.T) {
	handler := withLogging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := TraceIDFromContext(r.Context()); got == "" {
			t.Fatalf("expected trace_id in request context")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Trace-Id"); got == "" {
		t.Fatalf("expected X-Trace-Id response header")
	}
}

func TestWithLoggingPreservesIncomingTraceID(t *testing.T) {
	handler := withLogging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := TraceIDFromContext(r.Context()); got != "trace-from-client" {
			t.Fatalf("expected propagated trace_id, got %s", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Trace-Id", "trace-from-client")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Trace-Id"); got != "trace-from-client" {
		t.Fatalf("expected response trace header to preserve input, got %s", got)
	}
}
