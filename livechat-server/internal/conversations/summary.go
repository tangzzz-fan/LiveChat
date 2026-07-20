package conversations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/domain"
)

// Service maintains conversation_summaries projection and provides the list API.
type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// UpdateOnNewMessage updates the conversation summary for all members.
// Called after a message is created.
func (s *Service) UpdateOnNewMessage(ctx context.Context, conversationID string, senderUserID int64, preview string) error {
	// Get all members
	rows, err := s.db.QueryContext(ctx,
		"SELECT user_id FROM conversation_members WHERE conversation_id=$1", conversationID)
	if err != nil {
		return fmt.Errorf("get members: %w", err)
	}
	defer rows.Close()

	var members []int64
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err != nil {
			return err
		}
		members = append(members, uid)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, uid := range members {
		unreadDelta := 1
		if uid == senderUserID {
			unreadDelta = 0
		}
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO conversation_summaries (user_id, conversation_id, last_message_preview, last_message_at, unread_count, updated_at)
			 VALUES ($1, $2, $3, NOW(), $4, NOW())
			 ON CONFLICT (user_id, conversation_id) DO UPDATE SET
			   last_message_preview = EXCLUDED.last_message_preview,
			   last_message_at = EXCLUDED.last_message_at,
			   unread_count = conversation_summaries.unread_count + $4,
			   updated_at = NOW()`,
			uid, conversationID, preview, unreadDelta)
		if err != nil {
			return fmt.Errorf("upsert summary for user %d: %w", uid, err)
		}
	}
	return nil
}

// MarkRead sets unread_count to 0 for a user in a conversation.
func (s *Service) MarkRead(ctx context.Context, userID int64, conversationID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE conversation_summaries
		 SET unread_count = 0, updated_at = NOW()
		 WHERE user_id = $1 AND conversation_id = $2`,
		userID, conversationID)
	return err
}

// List returns the conversation list for a user, sorted by last_message_at DESC.
func (s *Service) List(ctx context.Context, userID int64, limit, offset int) ([]domain.ConversationSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT cs.user_id, cs.conversation_id, c.type,
		        cs.last_message_preview, cs.last_message_at,
		        cs.unread_count, cs.is_pinned, cs.updated_at
		 FROM conversation_summaries cs
		 JOIN conversations c ON c.id = cs.conversation_id
		 WHERE cs.user_id = $1
		   AND (cs.is_hidden IS FALSE OR cs.is_hidden IS NULL)
		 ORDER BY cs.is_pinned DESC, cs.last_message_at DESC NULLS LAST
		 LIMIT $2 OFFSET $3`,
		userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list summaries: %w", err)
	}
	defer rows.Close()

	var summaries []domain.ConversationSummary
	for rows.Next() {
		var s domain.ConversationSummary
		var lastMsgAt sql.NullTime
		if err := rows.Scan(&s.UserID, &s.ConversationID, &s.ConversationType,
			&s.LastMessagePreview, &lastMsgAt,
			&s.UnreadCount, &s.IsPinned, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan summary: %w", err)
		}
		if lastMsgAt.Valid {
			s.LastMessageAt = lastMsgAt.Time
		}
		summaries = append(summaries, s)
	}
	if summaries == nil {
		summaries = make([]domain.ConversationSummary, 0)
	}
	return summaries, rows.Err()
}
