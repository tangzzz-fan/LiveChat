package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	HTTPRequestsTotal     atomic.Int64
	MessagesSentTotal     atomic.Int64
	OutboxEventsCreated   atomic.Int64
	OutboxPendingCount    atomic.Int64
	OutboxProcessingCount atomic.Int64
	OutboxDoneCount       atomic.Int64
	OutboxFailedCount     atomic.Int64
	WSActiveConnections   atomic.Int64
	WSConnectionsTotal    atomic.Int64
	WSHeartbeatTimeouts   atomic.Int64
)

type requestSeries struct {
	Count     int64
	Durations []float64
}

var (
	requestMu       sync.Mutex
	requestCounts   = make(map[string]int64)
	requestDuration = make(map[string]*requestSeries)
)

func ObserveHTTPRequest(method, path string, status int, duration time.Duration) {
	HTTPRequestsTotal.Add(1)

	key := requestMetricKey(method, path, status)
	requestMu.Lock()
	defer requestMu.Unlock()
	requestCounts[key]++
	series := requestDuration[key]
	if series == nil {
		series = &requestSeries{}
		requestDuration[key] = series
	}
	series.Count++
	series.Durations = append(series.Durations, duration.Seconds())
}

func Handler(extraCollectors ...func() map[string]int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprint(w, render(extraCollectors...))
	})
}

func render(extraCollectors ...func() map[string]int64) string {
	var b strings.Builder
	overrides := make(map[string]int64)
	for _, collect := range extraCollectors {
		for name, value := range collect() {
			overrides[name] = value
		}
	}

	writeRequests(&b)
	writeGauge(&b, "messages_sent_total", float64(MessagesSentTotal.Load()))
	writeGauge(&b, "outbox_events_created_total", float64(OutboxEventsCreated.Load()))
	writeMetric(&b, overrides, "outbox_pending_count", float64(OutboxPendingCount.Load()))
	writeMetric(&b, overrides, "outbox_processing_count", float64(OutboxProcessingCount.Load()))
	writeMetric(&b, overrides, "outbox_failed_count", float64(OutboxFailedCount.Load()))
	writeMetric(&b, overrides, "outbox_consumer_lag_seconds", 0)
	writeGauge(&b, "ws_connections_active", float64(WSActiveConnections.Load()))
	writeGauge(&b, "ws_connections_total", float64(WSConnectionsTotal.Load()))
	writeGauge(&b, "ws_heartbeat_timeouts_total", float64(WSHeartbeatTimeouts.Load()))

	for name, value := range overrides {
		if strings.HasPrefix(name, "outbox_") {
			continue
		}
		writeGauge(&b, name, float64(value))
	}
	return b.String()
}

func writeRequests(b *strings.Builder) {
	requestMu.Lock()
	defer requestMu.Unlock()

	keys := make([]string, 0, len(requestCounts))
	for key := range requestCounts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		method, path, status := splitRequestMetricKey(key)
		labels := fmt.Sprintf(`method="%s",path="%s",status="%s"`, method, path, status)
		fmt.Fprintf(b, "http_requests_total{%s} %d\n", labels, requestCounts[key])

		series := requestDuration[key]
		if series == nil || len(series.Durations) == 0 {
			continue
		}
		for _, quantile := range []float64{0.5, 0.9, 0.99} {
			fmt.Fprintf(b,
				"http_request_duration_seconds{method=\"%s\",path=\"%s\",quantile=\"%.2g\"} %.6f\n",
				method, path, quantile, percentile(series.Durations, quantile),
			)
		}
	}
}

func writeGauge(b *strings.Builder, name string, value float64) {
	fmt.Fprintf(b, "%s %.6f\n", name, value)
}

func writeMetric(b *strings.Builder, overrides map[string]int64, name string, fallback float64) {
	if value, ok := overrides[name]; ok {
		writeGauge(b, name, float64(value))
		return
	}
	writeGauge(b, name, fallback)
}

func requestMetricKey(method, path string, status int) string {
	return fmt.Sprintf("%s|%s|%d", method, path, status)
}

func splitRequestMetricKey(key string) (string, string, string) {
	parts := strings.SplitN(key, "|", 3)
	if len(parts) != 3 {
		return "unknown", "unknown", "0"
	}
	return parts[0], parts[1], parts[2]
}

func percentile(values []float64, q float64) float64 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	if len(sorted) == 1 {
		return sorted[0]
	}
	idx := int(float64(len(sorted)-1) * q)
	return sorted[idx]
}
