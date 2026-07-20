package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"sync"
	"time"
)

var (
	sleepFn   = time.Sleep
	randFloat = rand.Float64
)

// Config holds consumer configuration.
type Config struct {
	PollInterval     time.Duration // 100ms active
	IdlePollInterval time.Duration // 500ms idle
	BatchSize        int           // 100
	MaxRetries       int           // 10
	WorkerCount      int           // 4
	LeaseTimeout     time.Duration // 60s
}

// Handler is called for each outbox event. Return an error to trigger retry.
type Handler func(ctx context.Context, event Event) error

// Event is a decoded outbox event row.
type Event struct {
	ID            int64
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       json.RawMessage
	Status        string
	RetryCount    int
	CreatedAt     time.Time
}

// Consumer polls the outbox_events table and dispatches to handlers.
type Consumer struct {
	db       *sql.DB
	cfg      Config
	handlers map[string]Handler
	mu       sync.RWMutex
}

// NewConsumer creates a Consumer with the given config.
func NewConsumer(db *sql.DB, cfg Config) *Consumer {
	c := &Consumer{
		db:       db,
		cfg:      cfg,
		handlers: make(map[string]Handler),
	}
	return c
}

// RegisterHandler associates a handler function with an event type.
func (c *Consumer) RegisterHandler(eventType string, h Handler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[eventType] = h
}

func (c *Consumer) getHandler(eventType string) (Handler, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	h, ok := c.handlers[eventType]
	return h, ok
}

// Run starts the consumer loop. Blocks until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context) error {
	slog.Info("outbox consumer starting",
		"workers", c.cfg.WorkerCount,
		"batch_size", c.cfg.BatchSize,
		"max_retries", c.cfg.MaxRetries,
	)

	// Reap stale processing events first
	if err := c.reapStale(ctx); err != nil {
		slog.Error("reap stale failed", "error", err)
	}

	// Start workers
	events := make(chan Event, c.cfg.BatchSize)
	var wg sync.WaitGroup
	workerCtx := context.WithoutCancel(ctx)
	for i := 0; i < c.cfg.WorkerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for event := range events {
				c.processEvent(workerCtx, workerID, event)
			}
		}(i)
	}

	// Main polling loop
	pollInterval := c.cfg.PollInterval
	for {
		select {
		case <-ctx.Done():
			close(events)
			wg.Wait()
			slog.Info("outbox consumer stopped")
			return ctx.Err()
		case <-time.After(pollInterval):
			batch, err := c.fetchPending(ctx)
			if err != nil {
				slog.Error("fetch pending", "error", err)
				continue
			}
			if len(batch) == 0 {
				pollInterval = c.cfg.IdlePollInterval
			} else {
				pollInterval = c.cfg.PollInterval
				for _, e := range batch {
					select {
					case events <- e:
					case <-ctx.Done():
						close(events)
						wg.Wait()
						return ctx.Err()
					}
				}
			}

			// Periodically reap stale processing events
			if time.Now().Second()%30 == 0 {
				c.reapStale(ctx)
			}
		}
	}
}

// fetchPending retrieves pending events up to BatchSize.
func (c *Consumer) fetchPending(ctx context.Context) ([]Event, error) {
	rows, err := c.db.QueryContext(ctx,
		`WITH claimed AS (
			SELECT id
			FROM outbox_events
			WHERE status = 'pending'
			ORDER BY created_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		),
		updated AS (
			UPDATE outbox_events o
			SET status = 'processing', processed_at = NOW()
			FROM claimed
			WHERE o.id = claimed.id
			RETURNING o.id, o.aggregate_type, o.aggregate_id, o.event_type, o.payload, o.status, o.retry_count, o.created_at
		)
		SELECT id, aggregate_type, aggregate_id, event_type, payload, status, retry_count, created_at
		FROM updated
		ORDER BY created_at`,
		c.cfg.BatchSize,
	)
	if err != nil {
		return nil, fmt.Errorf("query pending: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var payload []byte
		if err := rows.Scan(&e.ID, &e.AggregateType, &e.AggregateID,
			&e.EventType, &payload, &e.Status, &e.RetryCount, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		e.Payload = payload
		events = append(events, e)
	}
	return events, rows.Err()
}

// processEvent handles a single event through its registered handler.
func (c *Consumer) processEvent(ctx context.Context, workerID int, event Event) {
	handler, ok := c.getHandler(event.EventType)
	if !ok {
		slog.Warn("no handler for event type", "event_type", event.EventType, "event_id", event.ID)
		// Mark as done anyway to avoid blocking the queue
		c.markDone(ctx, event.ID)
		return
	}

	start := time.Now()
	err := handler(ctx, event)
	latency := time.Since(start)

	if err != nil {
		slog.Error("handler failed",
			"event_id", event.ID,
			"event_type", event.EventType,
			"aggregate_id", event.AggregateID,
			"retry_count", event.RetryCount,
			"latency_ms", latency.Milliseconds(),
			"error", err,
		)

		newRetryCount := event.RetryCount + 1
		if newRetryCount >= c.cfg.MaxRetries {
			c.markFailed(ctx, event.ID, err.Error())
			slog.Error("event moved to dead",
				"event_id", event.ID,
				"event_type", event.EventType,
				"retry_count", newRetryCount,
			)
			return
		}

		// Exponential backoff with jitter
		backoff := backoffDuration(newRetryCount)
		slog.Info("retrying event",
			"event_id", event.ID,
			"retry_count", newRetryCount,
			"backoff", backoff,
		)
		sleepFn(backoff)
		c.markRetry(ctx, event.ID, err.Error())
	} else {
		c.markDone(ctx, event.ID)
		slog.Debug("event processed",
			"event_id", event.ID,
			"event_type", event.EventType,
			"latency_ms", latency.Milliseconds(),
		)
	}
}

// backoffDuration returns min(1s * 2^retry, 30s) with ±25% jitter.
func backoffDuration(retryCount int) time.Duration {
	baseSeconds := math.Min(math.Pow(2, float64(retryCount-1)), 30)
	base := time.Duration(baseSeconds * float64(time.Second))
	jitter := time.Duration(float64(base) * 0.25 * (randFloat()*2 - 1))
	return base + jitter
}

// ── Status updates ───────────────────────────────────

func (c *Consumer) markDone(ctx context.Context, id int64) error {
	_, err := c.db.ExecContext(ctx,
		`UPDATE outbox_events SET status='done', processed_at=NOW() WHERE id=$1`, id)
	return err
}

func (c *Consumer) markRetry(ctx context.Context, id int64, lastErr string) error {
	_, err := c.db.ExecContext(ctx,
		`UPDATE outbox_events SET status='pending', retry_count=retry_count+1, last_error=$2, processed_at=NULL WHERE id=$1`, id, lastErr)
	return err
}

func (c *Consumer) markFailed(ctx context.Context, id int64, lastErr string) error {
	_, err := c.db.ExecContext(ctx,
		`UPDATE outbox_events SET status='failed', retry_count=retry_count+1, last_error=$2 WHERE id=$1`, id, lastErr)
	return err
}

func (c *Consumer) reapStale(ctx context.Context) error {
	res, err := c.db.ExecContext(ctx,
		`UPDATE outbox_events
		 SET status='pending', processed_at=NULL
		 WHERE status='processing'
		   AND processed_at < NOW() - INTERVAL '1 second' * $1`, int(c.cfg.LeaseTimeout.Seconds()))
	if err != nil {
		return fmt.Errorf("reap stale: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		slog.Info("reaped stale processing events", "count", n)
	}
	return nil
}

// ── Metrics ──────────────────────────────────────────

// Metrics returns current consumer metrics.
func (c *Consumer) Metrics(ctx context.Context) map[string]int64 {
	metrics := make(map[string]int64)
	rows := [][2]string{
		{"outbox_pending_count", "pending"},
		{"outbox_processing_count", "processing"},
		{"outbox_retry_count", "retry"},
		{"outbox_failed_count", "failed"},
		{"outbox_done_count", "done"},
	}
	for _, r := range rows {
		var count int64
		c.db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM outbox_events WHERE status=$1", r[1]).Scan(&count)
		metrics[r[0]] = count
	}

	// Consumer lag: oldest pending event age in seconds
	var lag sql.NullFloat64
	c.db.QueryRowContext(ctx,
		"SELECT EXTRACT(EPOCH FROM NOW() - MIN(created_at)) FROM outbox_events WHERE status = 'pending'",
	).Scan(&lag)
	if lag.Valid {
		metrics["outbox_consumer_lag_seconds"] = int64(lag.Float64)
	}
	return metrics
}
