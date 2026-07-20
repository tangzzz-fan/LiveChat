package outbox

import (
	"context"
	"database/sql"
	"errors"
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
	if len(events) != 1 {
		t.Fatalf("expected 1 claimed event, got %d", len(events))
	}
	if events[0].Status != "processing" {
		t.Fatalf("expected claimed event status processing, got %s", events[0].Status)
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
	if len(events) != 1 {
		t.Fatalf("expected 1 event claimed for recovery, got %d", len(events))
	}
	if events[0].ID != id {
		t.Fatalf("expected event %d to be retried, got %d", id, events[0].ID)
	}
	if events[0].RetryCount != 1 {
		t.Fatalf("expected claimed retry_count 1, got %d", events[0].RetryCount)
	}

	consumer.processEvent(ctx, 0, events[0])

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
