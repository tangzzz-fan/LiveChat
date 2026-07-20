package receipts

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/conversations"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/infra"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/sync"
)

func TestProcessReadAckCreatesOutboxAndProjectsReadState(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	const (
		readerUserID = int64(91002)
		peerUserID   = int64(91001)
		convID       = "test-read-ack-conv"
	)
	cleanupReadAckFixture(t, db, convID, peerUserID, readerUserID)
	t.Cleanup(func() {
		cleanupReadAckFixture(t, db, convID, peerUserID, readerUserID)
	})

	seedReadAckFixture(t, db, convID, peerUserID, readerUserID)

	svc := NewService(db, sync.NewService(db), conversations.NewService(db))
	err := svc.ProcessAck(ctx, AckRequest{
		UserID:         readerUserID,
		DeviceID:       "ios-reader",
		AckType:        "read",
		ConversationID: convID,
		LastReadSeq:    3,
		AckedAtMs:      time.Now().UnixMilli(),
	})
	if err != nil {
		t.Fatalf("ProcessAck: %v", err)
	}

	var payloadRaw []byte
	err = db.QueryRowContext(ctx,
		`SELECT payload
		 FROM outbox_events
		 WHERE aggregate_type='receipt' AND event_type='read_receipt' AND aggregate_id=$1
		 ORDER BY id DESC
		 LIMIT 1`,
		readReceiptAggregateID(convID, readerUserID, 3),
	).Scan(&payloadRaw)
	if err != nil {
		t.Fatalf("query outbox event: %v", err)
	}

	var payload ReadReceiptPayload
	if err := json.Unmarshal(payloadRaw, &payload); err != nil {
		t.Fatalf("json.Unmarshal payload: %v", err)
	}
	if payload.ConversationID != convID {
		t.Fatalf("unexpected conversation_id: got %s", payload.ConversationID)
	}
	if payload.LastReadSeq != 3 {
		t.Fatalf("unexpected last_read_seq: got %d", payload.LastReadSeq)
	}

	if err := svc.ConsumeReadReceipt(ctx, payload); err != nil {
		t.Fatalf("ConsumeReadReceipt: %v", err)
	}

	var unreadCount int
	err = db.QueryRowContext(ctx,
		`SELECT unread_count
		 FROM conversation_summaries
		 WHERE user_id=$1 AND conversation_id=$2`,
		readerUserID, convID,
	).Scan(&unreadCount)
	if err != nil {
		t.Fatalf("query unread_count: %v", err)
	}
	if unreadCount != 0 {
		t.Fatalf("expected unread_count=0, got %d", unreadCount)
	}

	var peerEventType string
	var peerPayloadRaw []byte
	err = db.QueryRowContext(ctx,
		`SELECT event_type, payload
		 FROM sync_events
		 WHERE user_id=$1 AND conversation_id=$2
		 ORDER BY event_seq DESC
		 LIMIT 1`,
		peerUserID, convID,
	).Scan(&peerEventType, &peerPayloadRaw)
	if err != nil {
		t.Fatalf("query peer sync event: %v", err)
	}
	if peerEventType != "message_read" {
		t.Fatalf("expected peer event message_read, got %s", peerEventType)
	}

	var peerPayload map[string]any
	if err := json.Unmarshal(peerPayloadRaw, &peerPayload); err != nil {
		t.Fatalf("json.Unmarshal peer payload: %v", err)
	}
	if int64(peerPayload["last_read_seq"].(float64)) != 3 {
		t.Fatalf("unexpected peer last_read_seq: %v", peerPayload["last_read_seq"])
	}

	var readerEventType string
	var readerPayloadRaw []byte
	err = db.QueryRowContext(ctx,
		`SELECT event_type, payload
		 FROM sync_events
		 WHERE user_id=$1 AND conversation_id=$2
		 ORDER BY event_seq DESC
		 LIMIT 1`,
		readerUserID, convID,
	).Scan(&readerEventType, &readerPayloadRaw)
	if err != nil {
		t.Fatalf("query reader sync event: %v", err)
	}
	if readerEventType != "conversation_updated" {
		t.Fatalf("expected reader event conversation_updated, got %s", readerEventType)
	}

	var readerPayload map[string]any
	if err := json.Unmarshal(readerPayloadRaw, &readerPayload); err != nil {
		t.Fatalf("json.Unmarshal reader payload: %v", err)
	}
	if int64(readerPayload["last_read_seq"].(float64)) != 3 {
		t.Fatalf("unexpected reader last_read_seq: %v", readerPayload["last_read_seq"])
	}
	if int64(readerPayload["unread_count"].(float64)) != 0 {
		t.Fatalf("unexpected reader unread_count: %v", readerPayload["unread_count"])
	}
}

func openTestDB(t *testing.T) *sql.DB {
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

func seedReadAckFixture(t *testing.T, db *sql.DB, convID string, peerUserID, readerUserID int64) {
	t.Helper()
	ctx := context.Background()

	mustExec(t, db, `INSERT INTO users (id, phone_e164, display_name) VALUES ($1, $2, $3)`, peerUserID, "+155591001", "peer")
	mustExec(t, db, `INSERT INTO users (id, phone_e164, display_name) VALUES ($1, $2, $3)`, readerUserID, "+155591002", "reader")
	mustExec(t, db, `INSERT INTO devices (id, user_id, platform, refresh_token_hash, last_seen_at) VALUES ($1, $2, $3, $4, NOW())`, "ios-reader", readerUserID, "ios", "hash-reader")
	mustExec(t, db, `INSERT INTO devices (id, user_id, platform, refresh_token_hash, last_seen_at) VALUES ($1, $2, $3, $4, NOW())`, "ios-reader-other", readerUserID, "ios", "hash-reader-other")
	mustExec(t, db, `INSERT INTO conversations (id, type) VALUES ($1, 'direct')`, convID)
	mustExec(t, db, `INSERT INTO conversation_members (conversation_id, user_id) VALUES ($1, $2), ($1, $3)`, convID, peerUserID, readerUserID)
	mustExec(t, db, `INSERT INTO conversation_summaries (user_id, conversation_id, last_message_preview, unread_count, updated_at) VALUES ($1, $2, 'hi', 0, NOW())`, peerUserID, convID)
	mustExec(t, db, `INSERT INTO conversation_summaries (user_id, conversation_id, last_message_preview, unread_count, updated_at) VALUES ($1, $2, 'hi', 3, NOW())`, readerUserID, convID)
	mustExec(t, db, `INSERT INTO messages (server_message_id, conversation_id, conversation_seq, sender_user_id, sender_device_id, client_message_id, message_type, content, server_received_at)
	 VALUES
	 ('msg-1', $1, 1, $2, 'ios-peer', 'c1', 'text', '{"text":"1"}', NOW()),
	 ('msg-2', $1, 2, $2, 'ios-peer', 'c2', 'text', '{"text":"2"}', NOW()),
	 ('msg-3', $1, 3, $2, 'ios-peer', 'c3', 'text', '{"text":"3"}', NOW())`,
		convID, peerUserID)

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("db.PingContext: %v", err)
	}
}

func cleanupReadAckFixture(t *testing.T, db *sql.DB, convID string, peerUserID, readerUserID int64) {
	t.Helper()
	mustExec(t, db, `DELETE FROM outbox_events WHERE aggregate_id LIKE $1`, convID+":%")
	mustExec(t, db, `DELETE FROM sync_events WHERE conversation_id=$1 OR user_id IN ($2, $3)`, convID, peerUserID, readerUserID)
	mustExec(t, db, `DELETE FROM conversation_summaries WHERE conversation_id=$1 OR user_id IN ($2, $3)`, convID, peerUserID, readerUserID)
	mustExec(t, db, `DELETE FROM messages WHERE conversation_id=$1`, convID)
	mustExec(t, db, `DELETE FROM conversation_members WHERE conversation_id=$1 OR user_id IN ($2, $3)`, convID, peerUserID, readerUserID)
	mustExec(t, db, `DELETE FROM conversations WHERE id=$1`, convID)
	mustExec(t, db, `DELETE FROM devices WHERE user_id IN ($1, $2)`, peerUserID, readerUserID)
	mustExec(t, db, `DELETE FROM users WHERE id IN ($1, $2)`, peerUserID, readerUserID)
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("ExecContext %q: %v", query, err)
	}
}
