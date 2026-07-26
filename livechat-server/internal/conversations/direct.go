package conversations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var (
	// ErrSelfConversation is returned when a user tries to open a 1:1
	// conversation with themselves.
	ErrSelfConversation = errors.New("cannot create a direct conversation with yourself")
	// ErrPeerNotFound is returned when the requested peer does not exist.
	ErrPeerNotFound = errors.New("peer user not found")
)

// DirectConversation is the result of EnsureDirect.
type DirectConversation struct {
	ConversationID string `json:"conversation_id"`
	Type           string `json:"type"`
	PeerUserID     int64  `json:"peer_user_id"`
	// Created distinguishes "just created" from "already existed", so clients
	// can tell an idempotent replay from a first-time open.
	Created bool `json:"created"`
}

// DirectConversationID derives a stable ID from an unordered user pair.
//
// Deriving instead of generating is what makes the API idempotent without a
// lookup table: both directions of the same pair map to the same row, so a
// concurrent create from either side collides on the primary key rather than
// producing two conversations for one chat.
func DirectConversationID(userA, userB int64) string {
	lo, hi := userA, userB
	if lo > hi {
		lo, hi = hi, lo
	}
	return fmt.Sprintf("conv_dm_%d_%d", lo, hi)
}

// EnsureDirect returns the 1:1 conversation between the two users, creating it
// on first call. Repeated calls return the same conversation with Created=false.
func (s *Service) EnsureDirect(ctx context.Context, userID, peerUserID int64) (*DirectConversation, error) {
	if userID == peerUserID {
		return nil, ErrSelfConversation
	}

	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, peerUserID).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("check peer: %w", err)
	}
	if !exists {
		return nil, ErrPeerNotFound
	}

	convID := DirectConversationID(userID, peerUserID)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var created bool
	err = tx.QueryRowContext(ctx,
		`INSERT INTO conversations (id, type, created_at)
		 VALUES ($1, 'direct', NOW())
		 ON CONFLICT (id) DO NOTHING
		 RETURNING TRUE`,
		convID,
	).Scan(&created)
	if errors.Is(err, sql.ErrNoRows) {
		created = false // already existed
	} else if err != nil {
		return nil, fmt.Errorf("insert conversation: %w", err)
	}

	for _, uid := range []int64{userID, peerUserID} {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO conversation_members (conversation_id, user_id, joined_at)
			 VALUES ($1, $2, NOW())
			 ON CONFLICT (conversation_id, user_id) DO NOTHING`,
			convID, uid,
		); err != nil {
			return nil, fmt.Errorf("insert member %d: %w", uid, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// Only the caller gets a summary row: an empty conversation should not
	// appear in the peer's list until there is something to show. The peer's
	// row is created by the summary projection on the first message.
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO conversation_summaries (user_id, conversation_id, last_message_preview, last_message_at, unread_count, updated_at)
		 VALUES ($1, $2, '', NOW(), 0, NOW())
		 ON CONFLICT (user_id, conversation_id) DO NOTHING`,
		userID, convID,
	)

	return &DirectConversation{
		ConversationID: convID,
		Type:           "direct",
		PeerUserID:     peerUserID,
		Created:        created,
	}, nil
}
