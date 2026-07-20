package sync

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/domain"
)

// Service handles sync event writing and querying.
type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// AppendEvent inserts a sync_event for the given user.
func (s *Service) AppendEvent(ctx context.Context, userID int64, eventType string, payload []byte) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sync_events (user_id, event_type, payload, created_at)
		 VALUES ($1, $2, $3, NOW())`,
		userID, eventType, string(payload),
	)
	if err != nil {
		return fmt.Errorf("append sync event: %w", err)
	}
	return nil
}

// AppendEventWithConv inserts a sync_event with a conversation_id.
func (s *Service) AppendEventWithConv(ctx context.Context, userID int64, conversationID, eventType string, payload []byte) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sync_events (user_id, conversation_id, event_type, payload, created_at)
		 VALUES ($1, $2, $3, $4, NOW())`,
		userID, conversationID, eventType, string(payload),
	)
	if err != nil {
		return fmt.Errorf("append sync event: %w", err)
	}
	return nil
}

// GetEvents returns sync events for a user after the given cursor.
func (s *Service) GetEvents(ctx context.Context, userID int64, cursor int64, limit int) ([]domain.SyncEvent, int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT event_seq, user_id, COALESCE(conversation_id, ''), event_type, payload, created_at
		 FROM sync_events
		 WHERE user_id=$1 AND event_seq > $2
		 ORDER BY event_seq
		 LIMIT $3`,
		userID, cursor, limit+1,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query sync events: %w", err)
	}
	defer rows.Close()

	var events []domain.SyncEvent
	for rows.Next() {
		var e domain.SyncEvent
		var payload []byte
		if err := rows.Scan(&e.EventSeq, &e.UserID, &e.ConversationID, &e.EventType, &payload, &e.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan sync event: %w", err)
		}
		e.Payload = string(payload)
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}

	// Get latest global event_seq for this user
	var latestSeq int64
	s.db.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(event_seq), 0) FROM sync_events WHERE user_id=$1", userID,
	).Scan(&latestSeq)

	return events, latestSeq, nil
}

// UpdateCursor upserts the sync cursor for a user+device.
func (s *Service) UpdateCursor(ctx context.Context, userID int64, deviceID string, lastEventSeq int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sync_cursors (user_id, device_id, last_event_seq, last_sync_at, updated_at)
		 VALUES ($1, $2, $3, NOW(), NOW())
		 ON CONFLICT (user_id, device_id) DO UPDATE
		 SET last_event_seq = GREATEST(sync_cursors.last_event_seq, $3),
		     last_sync_at = NOW(),
		     updated_at = NOW()`,
		userID, deviceID, lastEventSeq,
	)
	return err
}

// GetCursor returns the current sync cursor for a user+device.
func (s *Service) GetCursor(ctx context.Context, userID int64, deviceID string) (int64, error) {
	var cursor int64
	err := s.db.QueryRowContext(ctx,
		"SELECT last_event_seq FROM sync_cursors WHERE user_id=$1 AND device_id=$2",
		userID, deviceID,
	).Scan(&cursor)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return cursor, err
}

// LatestEventSeq returns the latest event_seq for a given user.
func (s *Service) LatestEventSeq(ctx context.Context, userID int64) (int64, error) {
	var latestSeq int64
	err := s.db.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(event_seq), 0) FROM sync_events WHERE user_id=$1",
		userID,
	).Scan(&latestSeq)
	if err != nil {
		return 0, fmt.Errorf("query latest event seq: %w", err)
	}
	return latestSeq, nil
}

// DeleteEventsOlderThan removes sync events older than the provided cutoff.
func (s *Service) DeleteEventsOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM sync_events WHERE created_at < $1`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("delete sync events older than cutoff: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected for sync cleanup: %w", err)
	}
	return rows, nil
}

// GetMessages returns messages for a conversation with cursor-based pagination.
func (s *Service) GetMessages(ctx context.Context, conversationID string, fromSeq int64, limit int) ([]domain.Message, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT server_message_id, conversation_id, conversation_seq,
		        sender_user_id, sender_device_id, client_message_id,
		        message_type, content, server_received_at
		 FROM messages
		 WHERE conversation_id=$1 AND conversation_seq >= $2
		 ORDER BY conversation_seq ASC
		 LIMIT $3`,
		conversationID, fromSeq, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var messages []domain.Message
	for rows.Next() {
		var m domain.Message
		var content []byte
		if err := rows.Scan(&m.ServerMessageID, &m.ConversationID, &m.ConversationSeq,
			&m.SenderUserID, &m.SenderDeviceID, &m.ClientMessageID,
			&m.MessageType, &content, &m.ServerReceivedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		m.Content = string(content)
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// VerifyMembership checks if a user is a member of a conversation.
func (s *Service) VerifyMembership(ctx context.Context, userID int64, conversationID string) (bool, error) {
	var ok bool
	err := s.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id=$1 AND user_id=$2)",
		conversationID, userID,
	).Scan(&ok)
	return ok, err
}
