package receipts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/conversations"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/domain"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/sync"
)

var ErrUnsupportedAckType = errors.New("unsupported ack type")

type Service struct {
	db      *sql.DB
	syncSvc *sync.Service
	convSvc *conversations.Service
}

type AckRequest struct {
	UserID         int64
	DeviceID       string
	AckType        string
	EventSeq       int64
	ConversationID string
	LastReadSeq    int64
	AckedAtMs      int64
}

type ReadReceiptPayload struct {
	ReaderUserID   int64  `json:"reader_user_id"`
	ReaderDeviceID string `json:"reader_device_id"`
	ConversationID string `json:"conversation_id"`
	LastReadSeq    int64  `json:"last_read_seq"`
	AckedAtMs      int64  `json:"acked_at_ms"`
}

func NewService(db *sql.DB, syncSvc *sync.Service, convSvc *conversations.Service) *Service {
	return &Service{
		db:      db,
		syncSvc: syncSvc,
		convSvc: convSvc,
	}
}

func (s *Service) ProcessAck(ctx context.Context, req AckRequest) error {
	switch req.AckType {
	case domain.ReceiptTypeRead:
		return s.enqueueReadReceipt(ctx, req)
	default:
		return ErrUnsupportedAckType
	}
}

func (s *Service) enqueueReadReceipt(ctx context.Context, req AckRequest) error {
	if req.ConversationID == "" {
		return fmt.Errorf("conversation_id is required")
	}
	if req.LastReadSeq <= 0 {
		return fmt.Errorf("last_read_seq must be positive")
	}

	var isMember bool
	if err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM conversation_members
			WHERE conversation_id=$1 AND user_id=$2
		)`,
		req.ConversationID, req.UserID,
	).Scan(&isMember); err != nil {
		return fmt.Errorf("verify membership: %w", err)
	}
	if !isMember {
		return fmt.Errorf("user %d is not a member of conversation %s", req.UserID, req.ConversationID)
	}

	payload := ReadReceiptPayload{
		ReaderUserID:   req.UserID,
		ReaderDeviceID: req.DeviceID,
		ConversationID: req.ConversationID,
		LastReadSeq:    req.LastReadSeq,
		AckedAtMs:      req.AckedAtMs,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal read receipt payload: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload, status, created_at)
		 VALUES ($1, $2, $3, $4, 'pending', NOW())`,
		domain.AggregateTypeReceipt,
		readReceiptAggregateID(req.ConversationID, req.UserID, req.LastReadSeq),
		"read_receipt",
		raw,
	)
	if err != nil {
		return fmt.Errorf("insert read receipt outbox event: %w", err)
	}
	return nil
}

func (s *Service) ConsumeReadReceipt(ctx context.Context, payload ReadReceiptPayload) error {
	if err := s.convSvc.MarkRead(ctx, payload.ReaderUserID, payload.ConversationID); err != nil {
		return fmt.Errorf("mark read: %w", err)
	}

	conversationUpdated, err := json.Marshal(map[string]any{
		"conversation_id": payload.ConversationID,
		"unread_count":    0,
		"last_read_seq":   payload.LastReadSeq,
	})
	if err != nil {
		return fmt.Errorf("marshal conversation_updated payload: %w", err)
	}
	if err := s.syncSvc.AppendEventWithConv(ctx, payload.ReaderUserID, payload.ConversationID, domain.EventTypeConversationUpdated, conversationUpdated); err != nil {
		return fmt.Errorf("append conversation_updated: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT user_id
		 FROM conversation_members
		 WHERE conversation_id=$1 AND user_id <> $2`,
		payload.ConversationID, payload.ReaderUserID,
	)
	if err != nil {
		return fmt.Errorf("query other members: %w", err)
	}
	defer rows.Close()

	messageRead, err := json.Marshal(map[string]any{
		"conversation_id": payload.ConversationID,
		"reader_user_id":  payload.ReaderUserID,
		"last_read_seq":   payload.LastReadSeq,
	})
	if err != nil {
		return fmt.Errorf("marshal message_read payload: %w", err)
	}

	for rows.Next() {
		var otherUserID int64
		if err := rows.Scan(&otherUserID); err != nil {
			return fmt.Errorf("scan other member: %w", err)
		}
		if err := s.syncSvc.AppendEventWithConv(ctx, otherUserID, payload.ConversationID, domain.EventTypeMessageRead, messageRead); err != nil {
			return fmt.Errorf("append message_read for user %d: %w", otherUserID, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate other members: %w", err)
	}
	return nil
}

func readReceiptAggregateID(conversationID string, userID, lastReadSeq int64) string {
	return fmt.Sprintf("%s:%d:%d", conversationID, userID, lastReadSeq)
}

func NormalizeAckedAt(ackedAtMs int64) int64 {
	if ackedAtMs > 0 {
		return ackedAtMs
	}
	return time.Now().UnixMilli()
}
