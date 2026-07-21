package group

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SyncWriter is the interface for appending sync events to a user's stream.
type SyncWriter interface {
	AppendEventWithConv(ctx context.Context, userID int64, conversationID, eventType string, payload []byte) error
}

// Service handles group CRUD and member management.
type Service struct {
	db        *sql.DB
	sync      SyncWriter
	convIDFn  func(groupID string) string // defaults to GetConversationID
}

func NewService(db *sql.DB) *Service {
	s := &Service{db: db}
	s.convIDFn = s.GetConversationID
	return s
}

// SetSyncWriter optionally enables sync-event projection for group membership changes.
func (s *Service) SetSyncWriter(sync SyncWriter) {
	s.sync = sync
}

// SetConversationIDFn overrides the default conv_<groupID> prefix.
func (s *Service) SetConversationIDFn(fn func(groupID string) string) {
	s.convIDFn = fn
}

// ── Types ──────────────────────────────────────────

type Group struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description,omitempty"`
	AvatarKey      string    `json:"avatar_key,omitempty"`
	CreatorUserID  int64     `json:"creator_user_id"`
	MaxMembers     int       `json:"max_members"`
	CurrentMembers int       `json:"current_members"`
	IsArchived     bool      `json:"is_archived"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Member struct {
	GroupID  string    `json:"group_id"`
	UserID   int64     `json:"user_id"`
	Role     string    `json:"role"` // "owner", "admin", "member"
	JoinedAt time.Time `json:"joined_at"`
	AddedBy  int64     `json:"added_by,omitempty"`
	IsMuted  bool      `json:"is_muted"`
}

// ── Sentinel errors ────────────────────────────────

var (
	ErrGroupNotFound     = errors.New("group not found")
	ErrNotOwner          = errors.New("only the group owner can perform this action")
	ErrNotAdmin          = errors.New("requires admin or owner privileges")
	ErrNotMember         = errors.New("user is not a group member")
	ErrGroupFull         = errors.New("group is full")
	ErrAlreadyMember     = errors.New("user is already a member")
	ErrCannotRemoveOwner = errors.New("cannot remove the group owner")
	ErrCannotRemoveSelf  = errors.New("owner cannot leave the group; transfer ownership first")
)

// ── Group CRUD ─────────────────────────────────────

// CreateGroup creates a new group and adds the creator as owner.
// It also creates the corresponding conversation and conversation_members entries.
func (s *Service) CreateGroup(ctx context.Context, name, description string, creatorUserID int64) (*Group, error) {
	groupID := fmt.Sprintf("grp_%d_%d", creatorUserID, time.Now().UnixNano()/1000000)
	convID := s.convIDFn(groupID)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Insert group
	var g Group
	err = tx.QueryRowContext(ctx,
		`INSERT INTO groups (id, name, description, creator_user_id, current_members, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, 1, NOW(), NOW())
		 RETURNING id, name, description, creator_user_id, max_members, current_members, is_archived, created_at, updated_at`,
		groupID, name, description, creatorUserID,
	).Scan(&g.ID, &g.Name, &g.Description, &g.CreatorUserID, &g.MaxMembers,
		&g.CurrentMembers, &g.IsArchived, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert group: %w", err)
	}

	// Create conversation
	_, err = tx.ExecContext(ctx,
		`INSERT INTO conversations (id, type, created_at) VALUES ($1, 'group', NOW())`,
		convID)
	if err != nil {
		return nil, fmt.Errorf("insert conversation: %w", err)
	}

	// Add creator as owner
	_, err = tx.ExecContext(ctx,
		`INSERT INTO group_members (group_id, user_id, role, joined_at) VALUES ($1, $2, 'owner', NOW())`,
		groupID, creatorUserID)
	if err != nil {
		return nil, fmt.Errorf("insert owner: %w", err)
	}

	// Add to conversation_members
	_, err = tx.ExecContext(ctx,
		`INSERT INTO conversation_members (conversation_id, user_id, joined_at) VALUES ($1, $2, NOW())`,
		convID, creatorUserID)
	if err != nil {
		return nil, fmt.Errorf("insert conv member: %w", err)
	}

	// Record group event
	_, err = tx.ExecContext(ctx,
		`INSERT INTO group_events (group_id, event_type, actor_user_id, created_at)
		 VALUES ($1, 'created', $2, NOW())`,
		groupID, creatorUserID)
	if err != nil {
		return nil, fmt.Errorf("insert group event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// Init conversation_summary for the creator (so the group appears in their list)
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO conversation_summaries (user_id, conversation_id, last_message_preview, last_message_at, unread_count, updated_at)
		 VALUES ($1, $2, '', NOW(), 0, NOW())
		 ON CONFLICT (user_id, conversation_id) DO NOTHING`,
		creatorUserID, convID,
	)

	return &g, nil
}

// GetGroup returns a group by ID.
func (s *Service) GetGroup(ctx context.Context, groupID string) (*Group, error) {
	var g Group
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, COALESCE(description,''), COALESCE(avatar_key,''),
		        creator_user_id, max_members, current_members, is_archived,
		        created_at, updated_at
		 FROM groups WHERE id=$1`, groupID,
	).Scan(&g.ID, &g.Name, &g.Description, &g.AvatarKey,
		&g.CreatorUserID, &g.MaxMembers, &g.CurrentMembers, &g.IsArchived,
		&g.CreatedAt, &g.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrGroupNotFound
	}
	return &g, err
}

// ── Member management ──────────────────────────────

// AddMembers adds users to a group. The caller must be an admin.
func (s *Service) AddMembers(ctx context.Context, groupID string, addedBy int64, userIDs []int64) error {
	g, err := s.GetGroup(ctx, groupID)
	if err != nil {
		return err
	}

	// Verify addedBy is admin/owner
	if err := s.verifyAdmin(ctx, groupID, addedBy); err != nil {
		return err
	}

	space := g.MaxMembers - g.CurrentMembers
	if len(userIDs) > space {
		return ErrGroupFull
	}

	convID := s.convIDFn(groupID)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	addedCount := 0
	for _, uid := range userIDs {
		// Check not already a member
		var exists bool
		tx.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM group_members WHERE group_id=$1 AND user_id=$2 AND left_at IS NULL)`,
			groupID, uid).Scan(&exists)
		if exists {
			continue // skip already-active members (idempotent)
		}

		// Check if previously left — if so, re-activate
		var previouslyLeft bool
		tx.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM group_members WHERE group_id=$1 AND user_id=$2 AND left_at IS NOT NULL)`,
			groupID, uid).Scan(&previouslyLeft)

		if previouslyLeft {
			_, err = tx.ExecContext(ctx,
				`UPDATE group_members SET role='member', left_at=NULL, joined_at=NOW(), added_by=$1
				 WHERE group_id=$2 AND user_id=$3`,
				addedBy, groupID, uid)
		} else {
			_, err = tx.ExecContext(ctx,
				`INSERT INTO group_members (group_id, user_id, role, joined_at, added_by)
				 VALUES ($1, $2, 'member', NOW(), $3)`,
				groupID, uid, addedBy)
		}
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("add member %d: %w", uid, err)
		}

		// Add to conversation_members
		_, err = tx.ExecContext(ctx,
			`INSERT INTO conversation_members (conversation_id, user_id, joined_at)
			 VALUES ($1, $2, NOW())
			 ON CONFLICT (conversation_id, user_id) DO NOTHING`,
			convID, uid)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("add conv member %d: %w", uid, err)
		}

		// Record group event
		_, err = tx.ExecContext(ctx,
			`INSERT INTO group_events (group_id, event_type, actor_user_id, target_user_id, created_at)
			 VALUES ($1, 'member_joined', $2, $3, NOW())`,
			groupID, addedBy, uid,
		)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("insert group event: %w", err)
		}

		addedCount++
	}

	if addedCount == 0 {
		tx.Rollback()
		return ErrAlreadyMember
	}

	// Update member count
	_, err = tx.ExecContext(ctx,
		`UPDATE groups SET current_members = current_members + $1, updated_at = NOW()
		 WHERE id=$2`, addedCount, groupID)
	if err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// After commit: create conversation_summary for new members and sync-events
	for _, uid := range userIDs {
		s.ensureSummary(ctx, uid, convID)
		s.emitMembershipEvent(ctx, uid, convID, "member_joined", groupID, addedBy, uid)
	}
	// Also emit to the adder
	s.emitMembershipEvent(ctx, addedBy, convID, "member_joined", groupID, addedBy, userIDs...)

	return nil
}

// RemoveMember removes a user from a group.
func (s *Service) RemoveMember(ctx context.Context, groupID string, removedBy, targetUserID int64) error {
	// Cannot remove the owner
	role, err := s.memberRole(ctx, groupID, targetUserID)
	if err != nil {
		return err
	}
	if role == "owner" {
		return ErrCannotRemoveOwner
	}

	// Verify removedBy has permission
	if err := s.verifyAdmin(ctx, groupID, removedBy); err != nil {
		return err
	}

	convID := s.convIDFn(groupID)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Mark as left
	result, err := tx.ExecContext(ctx,
		`UPDATE group_members SET left_at = NOW() WHERE group_id=$1 AND user_id=$2 AND left_at IS NULL`,
		groupID, targetUserID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotMember
	}

	// Remove from conversation_members
	tx.ExecContext(ctx,
		`DELETE FROM conversation_members WHERE conversation_id=$1 AND user_id=$2`,
		convID, targetUserID)

	// Update member count
	tx.ExecContext(ctx,
		`UPDATE groups SET current_members = current_members - 1, updated_at = NOW() WHERE id=$1`,
		groupID)

	// Record group event
	tx.ExecContext(ctx,
		`INSERT INTO group_events (group_id, event_type, actor_user_id, target_user_id, created_at)
		 VALUES ($1, 'member_removed', $2, $3, NOW())`,
		groupID, removedBy, targetUserID,
	)

	if err := tx.Commit(); err != nil {
		return err
	}

	// Hide conversation summary for removed user
	_, _ = s.db.ExecContext(ctx,
		`UPDATE conversation_summaries SET is_hidden = TRUE, updated_at = NOW()
		 WHERE user_id=$1 AND conversation_id=$2`,
		targetUserID, convID,
	)

	// Emit event to remaining members + removed user
	allMembers, _ := s.GetMembers(ctx, groupID)
	for _, m := range allMembers {
		s.emitMembershipEvent(ctx, m.UserID, convID, "member_removed", groupID, removedBy, targetUserID)
	}
	// Also emit to the removed user
	s.emitMembershipEvent(ctx, targetUserID, convID, "member_removed", groupID, removedBy, targetUserID)

	return nil
}

// LeaveGroup allows a member to voluntarily leave a group.
func (s *Service) LeaveGroup(ctx context.Context, groupID string, userID int64) error {
	role, err := s.memberRole(ctx, groupID, userID)
	if err != nil {
		return err
	}
	if role == "owner" {
		return ErrCannotRemoveSelf
	}

	convID := s.convIDFn(groupID)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx,
		`UPDATE group_members SET left_at = NOW() WHERE group_id=$1 AND user_id=$2 AND left_at IS NULL`,
		groupID, userID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotMember
	}

	tx.ExecContext(ctx,
		`DELETE FROM conversation_members WHERE conversation_id=$1 AND user_id=$2`,
		convID, userID)

	tx.ExecContext(ctx,
		`UPDATE groups SET current_members = current_members - 1, updated_at = NOW() WHERE id=$1`,
		groupID)

	tx.ExecContext(ctx,
		`INSERT INTO group_events (group_id, event_type, actor_user_id, target_user_id, created_at)
		 VALUES ($1, 'member_left', $2, $3, NOW())`,
		groupID, userID, userID,
	)

	if err := tx.Commit(); err != nil {
		return err
	}

	// Hide conversation summary
	_, _ = s.db.ExecContext(ctx,
		`UPDATE conversation_summaries SET is_hidden = TRUE, updated_at = NOW()
		 WHERE user_id=$1 AND conversation_id=$2`,
		userID, convID,
	)

	// Emit event to user who left + remaining members
	s.emitMembershipEvent(ctx, userID, convID, "member_left", groupID, userID, userID)
	allMembers, _ := s.GetMembers(ctx, groupID)
	for _, m := range allMembers {
		s.emitMembershipEvent(ctx, m.UserID, convID, "member_left", groupID, userID, userID)
	}

	return nil
}

// GetMembers returns all active members of a group.
func (s *Service) GetMembers(ctx context.Context, groupID string) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT group_id, user_id, role, joined_at, COALESCE(added_by, 0), is_muted
		 FROM group_members
		 WHERE group_id=$1 AND left_at IS NULL
		 ORDER BY joined_at`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.GroupID, &m.UserID, &m.Role, &m.JoinedAt, &m.AddedBy, &m.IsMuted); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// GetConversationID returns the conversation ID for a group.
func (s *Service) GetConversationID(groupID string) string {
	return fmt.Sprintf("conv_%s", groupID)
}

// ── Helpers ────────────────────────────────────────

func (s *Service) verifyAdmin(ctx context.Context, groupID string, userID int64) error {
	role, err := s.memberRole(ctx, groupID, userID)
	if err != nil {
		return err
	}
	if role != "owner" && role != "admin" {
		return ErrNotAdmin
	}
	return nil
}

func (s *Service) memberRole(ctx context.Context, groupID string, userID int64) (string, error) {
	var role string
	err := s.db.QueryRowContext(ctx,
		"SELECT role FROM group_members WHERE group_id=$1 AND user_id=$2 AND left_at IS NULL",
		groupID, userID).Scan(&role)
	if err == sql.ErrNoRows {
		return "", ErrNotMember
	}
	return role, err
}

func (s *Service) ensureSummary(ctx context.Context, userID int64, conversationID string) {
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO conversation_summaries (user_id, conversation_id, last_message_preview, last_message_at, unread_count, updated_at)
		 VALUES ($1, $2, '', NOW(), 0, NOW())
		 ON CONFLICT (user_id, conversation_id) DO NOTHING`,
		userID, conversationID,
	)
}

func (s *Service) emitMembershipEvent(ctx context.Context, userID int64, conversationID, eventType, groupID string, actorUserID int64, targetUserIDs ...int64) {
	if s.sync == nil {
		return
	}
	payload := fmt.Sprintf(`{"group_id":"%s","actor_user_id":%d,"event_type":"%s"}`, groupID, actorUserID, eventType)
	_ = s.sync.AppendEventWithConv(ctx, userID, conversationID, eventType, []byte(payload))
}
