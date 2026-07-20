package metrics

import (
	"expvar"
	"net/http"
	"sync/atomic"
)

var (
	HTTPRequestsTotal   atomic.Int64
	MessagesSentTotal   atomic.Int64
	OutboxEventsCreated atomic.Int64
	OutboxPendingCount  atomic.Int64
	OutboxDoneCount     atomic.Int64
	OutboxFailedCount   atomic.Int64
	WSActiveConnections atomic.Int64
	WSHeartbeatTimeouts atomic.Int64
)

func init() {
	expvar.Publish("http_requests_total", expvar.Func(func() interface{} {
		return HTTPRequestsTotal.Load()
	}))
	expvar.Publish("messages_sent_total", expvar.Func(func() interface{} {
		return MessagesSentTotal.Load()
	}))
	expvar.Publish("outbox_events_created_total", expvar.Func(func() interface{} {
		return OutboxEventsCreated.Load()
	}))
	expvar.Publish("outbox_pending_count", expvar.Func(func() interface{} {
		return OutboxPendingCount.Load()
	}))
	expvar.Publish("outbox_done_count", expvar.Func(func() interface{} {
		return OutboxDoneCount.Load()
	}))
	expvar.Publish("outbox_failed_count", expvar.Func(func() interface{} {
		return OutboxFailedCount.Load()
	}))
	expvar.Publish("ws_active_connections", expvar.Func(func() interface{} {
		return WSActiveConnections.Load()
	}))
	expvar.Publish("ws_heartbeat_timeouts_total", expvar.Func(func() interface{} {
		return WSHeartbeatTimeouts.Load()
	}))
}

// MetricsHandler serves expvar metrics in Prometheus-compatible format.
func Handler() http.Handler {
	return expvar.Handler()
}
