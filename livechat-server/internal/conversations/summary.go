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
		var summary domain.ConversationSummary
		var lastMsgAt sql.NullTime
		if err := rows.Scan(&summary.UserID, &summary.ConversationID, &summary.ConversationType,
			&summary.LastMessagePreview, &lastMsgAt,
			&summary.UnreadCount, &summary.IsPinned, &summary.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan summary: %w", err)
		}
		if lastMsgAt.Valid {
			summary.LastMessageAt = lastMsgAt.Time
		}
		members, err := s.listMembers(ctx, summary.ConversationID)
		if err != nil {
			return nil, fmt.Errorf("list members for %s: %w", summary.ConversationID, err)
		}
		summary.Members = members
		summaries = append(summaries, summary)
	}
	if summaries == nil {
		summaries = make([]domain.ConversationSummary, 0)
	}
	return summaries, rows.Err()
}

func (s *Service) listMembers(ctx context.Context, conversationID string) ([]domain.ConversationSummaryMember, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT cm.user_id, u.display_name
		 FROM conversation_members cm
		 JOIN users u ON u.id = cm.user_id
		 WHERE cm.conversation_id = $1
		 ORDER BY cm.user_id`,
		conversationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := make([]domain.ConversationSummaryMember, 0)
	for rows.Next() {
		var member domain.ConversationSummaryMember
		if err := rows.Scan(&member.UserID, &member.DisplayName); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}
