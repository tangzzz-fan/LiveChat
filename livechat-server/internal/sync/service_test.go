package sync

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/infra"
)

func TestGetEventsPaginatesAndReturnsLatestSeq(t *testing.T) {
	db := openSyncTestDB(t)
	ctx := context.Background()

	const userID = int64(93001)
	cleanupSyncFixture(t, db, userID, "ios-sync")
	t.Cleanup(func() {
		cleanupSyncFixture(t, db, userID, "ios-sync")
	})

	svc := NewService(db)
	for i := 1; i <= 3; i++ {
		payload := []byte(fmt.Sprintf(`{"n":%d}`, i))
		if err := svc.AppendEventWithConv(ctx, userID, "conv-sync", "message_created", payload); err != nil {
			t.Fatalf("AppendEventWithConv #%d: %v", i, err)
		}
	}

	events, latestSeq, err := svc.GetEvents(ctx, userID, 0, 2)
	if err != nil {
		t.Fatalf("GetEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected first page size 2, got %d", len(events))
	}
	if events[0].EventSeq >= events[1].EventSeq {
		t.Fatalf("expected ascending event_seq order, got %d then %d", events[0].EventSeq, events[1].EventSeq)
	}
	if latestSeq <= events[len(events)-1].EventSeq {
		t.Fatalf("expected latest_event_seq to be ahead of page cursor, got latest=%d last_page=%d", latestSeq, events[len(events)-1].EventSeq)
	}

	remaining, latestAfterCursor, err := svc.GetEvents(ctx, userID, events[len(events)-1].EventSeq, 2)
	if err != nil {
		t.Fatalf("GetEvents after cursor: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected one remaining event, got %d", len(remaining))
	}
	if remaining[0].EventSeq != latestSeq {
		t.Fatalf("expected remaining event seq %d, got %d", latestSeq, remaining[0].EventSeq)
	}
	if latestAfterCursor != latestSeq {
		t.Fatalf("expected stable latest_event_seq %d, got %d", latestSeq, latestAfterCursor)
	}
}

func TestUpdateCursorMonotonic(t *testing.T) {
	db := openSyncTestDB(t)
	ctx := context.Background()

	const (
		userID   = int64(93002)
		deviceID = "ios-sync"
	)
	cleanupSyncFixture(t, db, userID, deviceID)
	t.Cleanup(func() {
		cleanupSyncFixture(t, db, userID, deviceID)
	})

	svc := NewService(db)
	if err := svc.UpdateCursor(ctx, userID, deviceID, 5); err != nil {
		t.Fatalf("UpdateCursor forward: %v", err)
	}
	if err := svc.UpdateCursor(ctx, userID, deviceID, 3); err != nil {
		t.Fatalf("UpdateCursor backward: %v", err)
	}
	got, err := svc.GetCursor(ctx, userID, deviceID)
	if err != nil {
		t.Fatalf("GetCursor: %v", err)
	}
	if got != 5 {
		t.Fatalf("expected monotonic cursor 5, got %d", got)
	}
}

func TestDeleteEventsOlderThanRemovesExpiredRowsOnly(t *testing.T) {
	db := openSyncTestDB(t)
	ctx := context.Background()

	const userID = int64(93003)
	cleanupSyncFixture(t, db, userID, "ios-retention")
	t.Cleanup(func() {
		cleanupSyncFixture(t, db, userID, "ios-retention")
	})

	oldID := seedSyncEvent(t, db, userID, time.Now().Add(-31*24*time.Hour))
	newID := seedSyncEvent(t, db, userID, time.Now().Add(-24*time.Hour))

	svc := NewService(db)
	deleted, err := svc.DeleteEventsOlderThan(ctx, time.Now().Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("DeleteEventsOlderThan: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected one deleted sync event, got %d", deleted)
	}
	assertSyncEventMissing(t, db, oldID)
	assertSyncEventPresent(t, db, newID)
}

func openSyncTestDB(t *testing.T) *sql.DB {
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

func cleanupSyncFixture(t *testing.T, db *sql.DB, userID int64, deviceID string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), `DELETE FROM sync_cursors WHERE user_id=$1 AND device_id=$2`, userID, deviceID); err != nil {
		t.Fatalf("delete sync_cursors: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `DELETE FROM sync_events WHERE user_id=$1`, userID); err != nil {
		t.Fatalf("delete sync_events: %v", err)
	}
}

func seedSyncEvent(t *testing.T, db *sql.DB, userID int64, createdAt time.Time) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(context.Background(),
		`INSERT INTO sync_events (user_id, conversation_id, event_type, payload, created_at)
		 VALUES ($1, 'conv-sync', 'message_created', '{"ok":true}', $2)
		 RETURNING event_seq`,
		userID, createdAt,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert sync event: %v", err)
	}
	return id
}

func assertSyncEventMissing(t *testing.T, db *sql.DB, eventSeq int64) {
	t.Helper()
	var exists bool
	if err := db.QueryRowContext(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM sync_events WHERE event_seq=$1)`,
		eventSeq,
	).Scan(&exists); err != nil {
		t.Fatalf("query missing sync event: %v", err)
	}
	if exists {
		t.Fatalf("expected sync event %d to be deleted", eventSeq)
	}
}

func assertSyncEventPresent(t *testing.T, db *sql.DB, eventSeq int64) {
	t.Helper()
	var exists bool
	if err := db.QueryRowContext(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM sync_events WHERE event_seq=$1)`,
		eventSeq,
	).Scan(&exists); err != nil {
		t.Fatalf("query present sync event: %v", err)
	}
	if !exists {
		t.Fatalf("expected sync event %d to remain", eventSeq)
	}
}
