package outbox

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/infra"
)

func TestBackoffDurationGrowthAndCap(t *testing.T) {
	originalRand := randFloat
	randFloat = func() float64 { return 0.5 }
	t.Cleanup(func() { randFloat = originalRand })

	cases := []struct {
		retry int
		want  time.Duration
	}{
		{retry: 1, want: time.Second},
		{retry: 2, want: 2 * time.Second},
		{retry: 3, want: 4 * time.Second},
		{retry: 4, want: 8 * time.Second},
		{retry: 5, want: 16 * time.Second},
		{retry: 6, want: 30 * time.Second},
		{retry: 7, want: 30 * time.Second},
	}

	for _, tc := range cases {
		if got := backoffDuration(tc.retry); got != tc.want {
			t.Fatalf("retry=%d: want %s, got %s", tc.retry, tc.want, got)
		}
	}
}

func TestFetchPendingClaimsEventsAsProcessing(t *testing.T) {
	db := openOutboxTestDB(t)
	ctx := context.Background()

	id := seedOutboxEvent(t, db, "pending", 0, time.Now())
	t.Cleanup(func() { cleanupOutboxEvent(t, db, id) })

	consumer := NewConsumer(db, Config{BatchSize: 10})
	events, err := consumer.fetchPending(ctx)
	if err != nil {
		t.Fatalf("fetchPending: %v", err)
	}
	var claimed *Event
	for i := range events {
		if events[i].ID == id {
			claimed = &events[i]
			break
		}
	}
	if claimed == nil {
		t.Fatalf("expected event %d among %d claimed events", id, len(events))
	}
	if claimed.Status != "processing" {
		t.Fatalf("expected claimed event status processing, got %s", claimed.Status)
	}

	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM outbox_events WHERE id=$1`, id).Scan(&status); err != nil {
		t.Fatalf("query claimed status: %v", err)
	}
	if status != "processing" {
		t.Fatalf("expected stored status processing, got %s", status)
	}
}

func TestProcessEventFailureReturnsEventToPending(t *testing.T) {
	db := openOutboxTestDB(t)
	ctx := context.Background()

	id := seedOutboxEvent(t, db, "processing", 0, time.Now())
	t.Cleanup(func() { cleanupOutboxEvent(t, db, id) })

	consumer := NewConsumer(db, Config{MaxRetries: 10})
	consumer.RegisterHandler("message_created", func(context.Context, Event) error {
		return errors.New("boom")
	})

	originalSleep := sleepFn
	originalRand := randFloat
	sleepFn = func(time.Duration) {}
	randFloat = func() float64 { return 0.5 }
	t.Cleanup(func() {
		sleepFn = originalSleep
		randFloat = originalRand
	})

	consumer.processEvent(ctx, 0, Event{
		ID:            id,
		AggregateType: "message",
		AggregateID:   "agg-1",
		EventType:     "message_created",
		Status:        "processing",
		RetryCount:    0,
	})

	var status string
	var retryCount int
	var processedAt sql.NullTime
	if err := db.QueryRowContext(ctx,
		`SELECT status, retry_count, processed_at FROM outbox_events WHERE id=$1`,
		id,
	).Scan(&status, &retryCount, &processedAt); err != nil {
		t.Fatalf("query retried event: %v", err)
	}
	if status != "pending" {
		t.Fatalf("expected status pending after retry, got %s", status)
	}
	if retryCount != 1 {
		t.Fatalf("expected retry_count 1, got %d", retryCount)
	}
	if processedAt.Valid {
		t.Fatalf("expected processed_at to be reset for retry")
	}
}

func TestProcessEventRetryThenRecoveryMarksDoneWithoutLoss(t *testing.T) {
	db := openOutboxTestDB(t)
	ctx := context.Background()

	id := seedOutboxEvent(t, db, "processing", 0, time.Now())
	t.Cleanup(func() { cleanupOutboxEvent(t, db, id) })

	consumer := NewConsumer(db, Config{BatchSize: 10, MaxRetries: 10})
	attempts := 0
	consumer.RegisterHandler("message_created", func(context.Context, Event) error {
		attempts++
		if attempts == 1 {
			return errors.New("transient downstream outage")
		}
		return nil
	})

	originalSleep := sleepFn
	originalRand := randFloat
	sleepFn = func(time.Duration) {}
	randFloat = func() float64 { return 0.5 }
	t.Cleanup(func() {
		sleepFn = originalSleep
		randFloat = originalRand
	})

	consumer.processEvent(ctx, 0, Event{
		ID:            id,
		AggregateType: "message",
		AggregateID:   "agg-1",
		EventType:     "message_created",
		Status:        "processing",
		RetryCount:    0,
	})

	assertOutboxStatus(t, db, id, "pending")

	var retryCount int
	var processedAt sql.NullTime
	if err := db.QueryRowContext(ctx,
		`SELECT retry_count, processed_at FROM outbox_events WHERE id=$1`,
		id,
	).Scan(&retryCount, &processedAt); err != nil {
		t.Fatalf("query retried event: %v", err)
	}
	if retryCount != 1 {
		t.Fatalf("expected retry_count 1 after transient failure, got %d", retryCount)
	}
	if processedAt.Valid {
		t.Fatalf("expected processed_at to be reset before retry claim")
	}

	events, err := consumer.fetchPending(ctx)
	if err != nil {
		t.Fatalf("fetchPending after retry: %v", err)
	}
	var claimed *Event
	for i := range events {
		if events[i].ID == id {
			claimed = &events[i]
			break
		}
	}
	if claimed == nil {
		t.Fatalf("expected event %d claimed for recovery among %d events", id, len(events))
	}
	if claimed.RetryCount != 1 {
		t.Fatalf("expected claimed retry_count 1, got %d", claimed.RetryCount)
	}

	consumer.processEvent(ctx, 0, *claimed)

	var status string
	if err := db.QueryRowContext(ctx,
		`SELECT status, retry_count, processed_at FROM outbox_events WHERE id=$1`,
		id,
	).Scan(&status, &retryCount, &processedAt); err != nil {
		t.Fatalf("query recovered event: %v", err)
	}
	if status != "done" {
		t.Fatalf("expected recovered event status done, got %s", status)
	}
	if retryCount != 1 {
		t.Fatalf("expected retry_count to stay 1 after recovery, got %d", retryCount)
	}
	if !processedAt.Valid {
		t.Fatalf("expected processed_at to be set after successful retry")
	}
	if attempts != 2 {
		t.Fatalf("expected handler to run twice, got %d attempts", attempts)
	}
}

func TestProcessEventMarksFailedAfterMaxRetries(t *testing.T) {
	db := openOutboxTestDB(t)
	ctx := context.Background()

	id := seedOutboxEvent(t, db, "processing", 0, time.Now())
	t.Cleanup(func() { cleanupOutboxEvent(t, db, id) })

	consumer := NewConsumer(db, Config{MaxRetries: 1})
	consumer.RegisterHandler("message_created", func(context.Context, Event) error {
		return errors.New("permanent failure")
	})

	consumer.processEvent(ctx, 0, Event{
		ID:            id,
		AggregateType: "message",
		AggregateID:   "agg-1",
		EventType:     "message_created",
		Status:        "processing",
		RetryCount:    0,
	})

	var status string
	var retryCount int
	if err := db.QueryRowContext(ctx,
		`SELECT status, retry_count FROM outbox_events WHERE id=$1`,
		id,
	).Scan(&status, &retryCount); err != nil {
		t.Fatalf("query failed event: %v", err)
	}
	if status != "failed" {
		t.Fatalf("expected status failed after max retries, got %s", status)
	}
	if retryCount != 1 {
		t.Fatalf("expected retry_count 1 after move to failed, got %d", retryCount)
	}
}

func TestRunProcessesMultipleEventsConcurrentlyWithoutLoss(t *testing.T) {
	db := openOutboxTestDB(t)
	ctx := context.Background()

	firstID := seedOutboxEvent(t, db, "pending", 0, time.Now())
	secondID := seedOutboxEvent(t, db, "pending", 0, time.Now().Add(time.Millisecond))
	t.Cleanup(func() {
		cleanupOutboxEvent(t, db, firstID)
		cleanupOutboxEvent(t, db, secondID)
	})

	consumer := NewConsumer(db, Config{
		PollInterval:     5 * time.Millisecond,
		IdlePollInterval: 5 * time.Millisecond,
		BatchSize:        10,
		MaxRetries:       10,
		WorkerCount:      2,
		LeaseTimeout:     60 * time.Second,
	})

	started := make(chan int64, 2)
	release := make(chan struct{})
	var processed atomic.Int32
	consumer.RegisterHandler("message_created", func(_ context.Context, event Event) error {
		started <- event.ID
		<-release
		processed.Add(1)
		return nil
	})

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- consumer.Run(runCtx)
	}()

	var seen [2]int64
	for i := range seen {
		select {
		case seen[i] = <-started:
		case <-time.After(2 * time.Second):
			t.Fatalf("expected both events to start processing")
		}
	}
	if (seen[0] != firstID && seen[1] != firstID) || (seen[0] != secondID && seen[1] != secondID) {
		t.Fatalf("expected both events to be processed, got %v", seen)
	}
	close(release)
	waitForOutboxCondition(t, func() bool {
		return processed.Load() == 2 &&
			outboxStatus(t, db, firstID) == "done" &&
			outboxStatus(t, db, secondID) == "done"
	})
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected consumer to stop with context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("consumer did not stop after cancellation")
	}
}

func TestRunGracefulShutdownWaitsForInflightEvent(t *testing.T) {
	db := openOutboxTestDB(t)
	ctx := context.Background()

	id := seedOutboxEvent(t, db, "pending", 0, time.Now())
	t.Cleanup(func() { cleanupOutboxEvent(t, db, id) })

	consumer := NewConsumer(db, Config{
		PollInterval:     5 * time.Millisecond,
		IdlePollInterval: 5 * time.Millisecond,
		BatchSize:        10,
		MaxRetries:       10,
		WorkerCount:      1,
		LeaseTimeout:     60 * time.Second,
	})

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	consumer.RegisterHandler("message_created", func(context.Context, Event) error {
		started <- struct{}{}
		<-release
		return nil
	})

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- consumer.Run(runCtx)
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatalf("handler did not start")
	}

	if status := outboxStatus(t, db, id); status != "processing" {
		t.Fatalf("expected event processing after handler start, got %s", status)
	}

	cancel()

	// Graceful shutdown must block on the in-flight handler (not return early).
	select {
	case err := <-done:
		t.Fatalf("consumer returned before inflight handler finished: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	if status := outboxStatus(t, db, id); status != "processing" {
		t.Fatalf("expected inflight event to remain processing before release, got %s", status)
	}

	close(release)

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected consumer to stop with context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("consumer did not stop after releasing inflight event")
	}

	assertOutboxStatus(t, db, id, "done")
}

func TestReapStaleResetsOnlyExpiredProcessingToPending(t *testing.T) {
	db := openOutboxTestDB(t)
	ctx := context.Background()

	staleID := seedOutboxEvent(t, db, "processing", 0, time.Now().Add(-2*time.Minute))
	freshID := seedOutboxEvent(t, db, "processing", 0, time.Now())
	doneID := seedOutboxEvent(t, db, "done", 0, time.Now().Add(-2*time.Minute))
	t.Cleanup(func() {
		cleanupOutboxEvent(t, db, staleID)
		cleanupOutboxEvent(t, db, freshID)
		cleanupOutboxEvent(t, db, doneID)
	})

	consumer := NewConsumer(db, Config{LeaseTimeout: 60 * time.Second})
	if err := consumer.reapStale(ctx); err != nil {
		t.Fatalf("reapStale: %v", err)
	}

	assertOutboxStatus(t, db, staleID, "pending")
	assertOutboxStatus(t, db, freshID, "processing")
	assertOutboxStatus(t, db, doneID, "done")
}

func openOutboxTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := infra.NewDB(infra.DBConfig{
		Host:            "localhost",
		Port:            5432,
		User:            "livechat",
		Password:        "livechat",
		Name:            "livechat",
		SSLMode:         "disable",
		MaxOpenConns:    2,
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func seedOutboxEvent(t *testing.T, db *sql.DB, status string, retryCount int, processedAt time.Time) int64 {
	t.Helper()

	var processedAtArg any
	if status == "processing" || status == "done" {
		processedAtArg = processedAt
	}

	var id int64
	err := db.QueryRowContext(context.Background(),
		`INSERT INTO outbox_events (
			aggregate_type, aggregate_id, event_type, payload, status, retry_count, processed_at
		) VALUES ('message', 'agg-1', 'message_created', '{}'::jsonb, $1, $2, $3)
		RETURNING id`,
		status, retryCount, processedAtArg,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert outbox event: %v", err)
	}
	return id
}

func cleanupOutboxEvent(t *testing.T, db *sql.DB, id int64) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), `DELETE FROM outbox_events WHERE id=$1`, id); err != nil {
		t.Fatalf("delete outbox event %d: %v", id, err)
	}
}

func assertOutboxStatus(t *testing.T, db *sql.DB, id int64, want string) {
	t.Helper()
	var got string
	if err := db.QueryRowContext(context.Background(), `SELECT status FROM outbox_events WHERE id=$1`, id).Scan(&got); err != nil {
		t.Fatalf("query outbox status %d: %v", id, err)
	}
	if got != want {
		t.Fatalf("event %d: want status %s, got %s", id, want, got)
	}
}

func outboxStatus(t *testing.T, db *sql.DB, id int64) string {
	t.Helper()
	var got string
	if err := db.QueryRowContext(context.Background(), `SELECT status FROM outbox_events WHERE id=$1`, id).Scan(&got); err != nil {
		t.Fatalf("query outbox status %d: %v", id, err)
	}
	return got
}

func waitForOutboxCondition(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not satisfied before timeout")
}
