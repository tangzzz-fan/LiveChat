package messages

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"fmt"
	"time"

	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/domain"
)

// Service handles the message send write path.
type Service struct {
	db       *sql.DB
	convUpdater ConversationUpdater
}

// ConversationUpdater is called after message insert to maintain conversation_summaries.
type ConversationUpdater interface {
	UpdateOnNewMessage(ctx context.Context, conversationID string, senderUserID int64, preview string) error
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// SetConversationUpdater wires the conversation summary updater (can be called after init).
func (s *Service) SetConversationUpdater(u ConversationUpdater) {
	s.convUpdater = u
}

// SendRequest carries the data needed to send a message.
type SendRequest struct {
	ClientMessageID string
	ConversationID  string
	SenderUserID    int64
	SenderDeviceID  string
	MessageType     string
	Content         string // JSON-encoded payload
}

// SendResult is returned after a successful (or idempotent) send.
type SendResult struct {
	ServerMessageID  string `json:"server_message_id"`
	ConversationSeq  int64  `json:"conversation_seq"`
	IsDuplicate      bool   `json:"is_duplicate"`
	ServerReceivedAt int64  `json:"server_received_at_ms"`
}

// Send validates, allocates seq, and writes message + outbox event atomically.
func (s *Service) Send(ctx context.Context, req SendRequest) (*SendResult, error) {
	// 1. Verify sender is a member of the conversation
	var isMember bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM conversation_members
			WHERE conversation_id=$1 AND user_id=$2
		)`, req.ConversationID, req.SenderUserID,
	).Scan(&isMember)
	if err != nil {
		return nil, fmt.Errorf("check membership: %w", err)
	}
	if !isMember {
		return nil, fmt.Errorf("%w: user %d is not a member of conversation %s",
			ErrNotMember, req.SenderUserID, req.ConversationID)
	}

	// 2. Allocate conversation_seq
	seq, err := s.ensureSeq(ctx, req.ConversationID)
	if err != nil {
		return nil, fmt.Errorf("allocate seq: %w", err)
	}

	// 3. Generate server_message_id
	serverMsgID := fmt.Sprintf("msg_%s_%06d", req.ConversationID, seq)

	// 4. Build outbox payload
	now := time.Now()
	payload := map[string]interface{}{
		"server_message_id": serverMsgID,
		"conversation_id":   req.ConversationID,
		"conversation_seq":  seq,
		"sender_user_id":    req.SenderUserID,
		"sender_device_id":  req.SenderDeviceID,
		"message_type":      req.MessageType,
		"content":           req.Content,
		"server_received_at_ms": now.UnixMilli(),
		"created_at":        now.Format(time.RFC3339Nano),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	// 5. Atomic write: message + outbox_event
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var isNew bool
	err = tx.QueryRowContext(ctx,
		`INSERT INTO messages (
			server_message_id, conversation_id, conversation_seq,
			sender_user_id, sender_device_id, client_message_id,
			message_type, content, server_received_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (sender_user_id, client_message_id) DO NOTHING
		RETURNING (xmax = 0) AS is_new`,
		serverMsgID, req.ConversationID, seq,
		req.SenderUserID, req.SenderDeviceID, req.ClientMessageID,
		req.MessageType, req.Content, now,
	).Scan(&isNew)

	if err == sql.ErrNoRows {
		// Conflict hit — fetch existing message to return its info
		var existingMsgID string
		var existingSeq int64
		var existingAt time.Time
		err2 := tx.QueryRowContext(ctx,
			`SELECT server_message_id, conversation_seq, server_received_at
			 FROM messages
			 WHERE sender_user_id=$1 AND client_message_id=$2`,
			req.SenderUserID, req.ClientMessageID,
		).Scan(&existingMsgID, &existingSeq, &existingAt)
		if err2 != nil {
			return nil, fmt.Errorf("fetch duplicate: %w", err2)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit (dup): %w", err)
		}
		return &SendResult{
			ServerMessageID:  existingMsgID,
			ConversationSeq:  existingSeq,
			IsDuplicate:      true,
			ServerReceivedAt: existingAt.UnixMilli(),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("insert message: %w", err)
	}

	if !isNew {
		// xmax != 0 means it was an update (shouldn't happen with DO NOTHING, but safety)
		return nil, fmt.Errorf("unexpected: message insert returned is_new=false without error")
	}

	// Insert outbox event in the same transaction
	_, err = tx.ExecContext(ctx,
		`INSERT INTO outbox_events (
			aggregate_type, aggregate_id, event_type, payload, status, created_at
		) VALUES ($1, $2, $3, $4, $5, $6)`,
		domain.AggregateTypeMessage,
		serverMsgID,
		domain.EventTypeMessageCreated,
		string(payloadJSON),
		domain.OutboxStatusPending,
		now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert outbox: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// Update conversation summary (outside tx — eventual consistency)
	if s.convUpdater != nil {
		preview := ""
		if req.MessageType == "text" {
			var content map[string]interface{}
			if json.Unmarshal([]byte(req.Content), &content) == nil {
				if t, ok := content["text"].(string); ok {
					preview = t
				}
			}
		}
		if err := s.convUpdater.UpdateOnNewMessage(context.Background(), req.ConversationID, req.SenderUserID, preview); err != nil {
			slog.Error("update conversation summary", "error", err)
		}
	}

	return &SendResult{
		ServerMessageID:  serverMsgID,
		ConversationSeq:  seq,
		IsDuplicate:      false,
		ServerReceivedAt: now.UnixMilli(),
	}, nil
}

// ensureSeq allocates the next conversation_seq for the given conversation,
// creating the PostgreSQL sequence if it does not yet exist.
func (s *Service) ensureSeq(ctx context.Context, conversationID string) (int64, error) {
	seqName := fmt.Sprintf("conversation_seq_%s", sanitizeSeqName(conversationID))

	// Try to create the sequence if it doesn't exist
	_, err := s.db.ExecContext(ctx,
		fmt.Sprintf("CREATE SEQUENCE IF NOT EXISTS %s CACHE 1", seqName),
	)
	if err != nil {
		return 0, fmt.Errorf("create seq %s: %w", seqName, err)
	}

	var seq int64
	err = s.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT nextval('%s')", seqName),
	).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("nextval: %w", err)
	}
	return seq, nil
}

// sanitizeSeqName replaces characters unsafe for PostgreSQL identifiers.
func sanitizeSeqName(id string) string {
	b := make([]byte, 0, len(id))
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			b = append(b, c)
		} else {
			b = append(b, '_')
		}
	}
	return string(b)
}

// ── Sentinel errors ────────────────────────────────

var ErrNotMember = fmt.Errorf("not a conversation member")
