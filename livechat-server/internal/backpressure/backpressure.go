package backpressure

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

// Defaults are deliberately conservative: a local learning setup should be able
// to absorb a normal burst without seeing 429, while a paused/stalled consumer
// (see docs/chaos/02-outbox-backpressure.md) crosses the threshold quickly.
const (
	DefaultPendingThreshold = 2000
	DefaultRetryAfter       = 5 * time.Second
	DefaultSampleInterval   = 2 * time.Second
)

// Config controls send-side backpressure. A non-positive PendingThreshold
// disables the limiter entirely.
type Config struct {
	PendingThreshold int64
	RetryAfter       time.Duration
	SampleInterval   time.Duration
}

// ConfigFromEnv reads overrides, falling back to the defaults above.
//
//	SEND_BACKPRESSURE_PENDING_THRESHOLD  int, <=0 disables
//	SEND_BACKPRESSURE_RETRY_AFTER_SEC    int
//	SEND_BACKPRESSURE_SAMPLE_MS          int
func ConfigFromEnv() Config {
	cfg := Config{
		PendingThreshold: DefaultPendingThreshold,
		RetryAfter:       DefaultRetryAfter,
		SampleInterval:   DefaultSampleInterval,
	}
	if v, ok := envInt("SEND_BACKPRESSURE_PENDING_THRESHOLD"); ok {
		cfg.PendingThreshold = int64(v)
	}
	if v, ok := envInt("SEND_BACKPRESSURE_RETRY_AFTER_SEC"); ok && v > 0 {
		cfg.RetryAfter = time.Duration(v) * time.Second
	}
	if v, ok := envInt("SEND_BACKPRESSURE_SAMPLE_MS"); ok && v > 0 {
		cfg.SampleInterval = time.Duration(v) * time.Millisecond
	}
	return cfg
}

func envInt(key string) (int, bool) {
	raw := os.Getenv(key)
	if raw == "" {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		slog.Warn("ignoring invalid backpressure env value", "key", key, "value", raw)
		return 0, false
	}
	return n, true
}

// Limiter answers "is the outbox too far behind to accept new writes?".
//
// The pending count is sampled in the background so the send path never pays
// for a COUNT(*); a stale sample by one interval is acceptable for a signal
// that only needs to be directionally correct.
type Limiter struct {
	db  *sql.DB
	cfg Config

	pending   atomic.Int64
	rejected  atomic.Int64
	sampledAt atomic.Int64 // unix milli of last successful sample
}

func NewLimiter(db *sql.DB, cfg Config) *Limiter {
	if cfg.RetryAfter <= 0 {
		cfg.RetryAfter = DefaultRetryAfter
	}
	if cfg.SampleInterval <= 0 {
		cfg.SampleInterval = DefaultSampleInterval
	}
	return &Limiter{db: db, cfg: cfg}
}

// Enabled reports whether the threshold is active.
func (l *Limiter) Enabled() bool {
	return l != nil && l.cfg.PendingThreshold > 0
}

// Run samples the pending backlog until ctx is cancelled.
func (l *Limiter) Run(ctx context.Context) {
	if !l.Enabled() {
		slog.Info("send backpressure disabled")
		return
	}
	slog.Info("send backpressure enabled",
		"pending_threshold", l.cfg.PendingThreshold,
		"retry_after", l.cfg.RetryAfter,
		"sample_interval", l.cfg.SampleInterval,
	)

	ticker := time.NewTicker(l.cfg.SampleInterval)
	defer ticker.Stop()

	l.sample(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.sample(ctx)
		}
	}
}

func (l *Limiter) sample(ctx context.Context) {
	var pending int64
	err := l.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM outbox_events WHERE status IN ('pending', 'processing')`,
	).Scan(&pending)
	if err != nil {
		// Fail open: a broken sample must not block sends.
		slog.Warn("backpressure sample failed", "error", err)
		return
	}
	l.SetPending(pending)
}

// SetPending overrides the current sample (used by Run and by tests).
func (l *Limiter) SetPending(pending int64) {
	l.pending.Store(pending)
	l.sampledAt.Store(time.Now().UnixMilli())
}

// Pending returns the most recent sample.
func (l *Limiter) Pending() int64 {
	if l == nil {
		return 0
	}
	return l.pending.Load()
}

// Rejected returns how many sends have been shed so far.
func (l *Limiter) Rejected() int64 {
	if l == nil {
		return 0
	}
	return l.rejected.Load()
}

// Allow reports whether a new send may proceed. When it returns false the
// caller should respond 429 with the returned Retry-After hint.
func (l *Limiter) Allow() (bool, time.Duration) {
	if !l.Enabled() {
		return true, 0
	}
	if l.pending.Load() < l.cfg.PendingThreshold {
		return true, 0
	}
	l.rejected.Add(1)
	return false, l.cfg.RetryAfter
}

// Metrics exposes gauges for the /metrics endpoint.
func (l *Limiter) Metrics() map[string]int64 {
	if l == nil {
		return nil
	}
	return map[string]int64{
		"send_backpressure_pending_sample": l.Pending(),
		"send_backpressure_threshold":      l.cfg.PendingThreshold,
		"send_backpressure_rejected_total": l.Rejected(),
	}
}
