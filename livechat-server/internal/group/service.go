package group

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Service handles group CRUD and member management.
type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
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
)

// ── Group CRUD ─────────────────────────────────────

// CreateGroup creates a new group and adds the creator as owner.
// It also creates the corresponding conversation and conversation_members entries.
func (s *Service) CreateGroup(ctx context.Context, name, description string, creatorUserID int64) (*Group, error) {
	groupID := fmt.Sprintf("grp_%d_%d", creatorUserID, time.Now().UnixNano()/1000000)
	convID := fmt.Sprintf("conv_%s", groupID)

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

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

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

	convID := fmt.Sprintf("conv_%s", groupID)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, uid := range userIDs {
		// Check not already a member
		var exists bool
		tx.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM group_members WHERE group_id=$1 AND user_id=$2 AND left_at IS NULL)`,
			groupID, uid).Scan(&exists)
		if exists {
			tx.Rollback()
			return fmt.Errorf("%w: user %d", ErrAlreadyMember, uid)
		}

		// Add to group_members
		_, err = tx.ExecContext(ctx,
			`INSERT INTO group_members (group_id, user_id, role, joined_at, added_by)
			 VALUES ($1, $2, 'member', NOW(), $3)`,
			groupID, uid, addedBy)
		if err != nil {
			tx.Rollback()
			return err
		}

		// Add to conversation_members
		_, err = tx.ExecContext(ctx,
			`INSERT INTO conversation_members (conversation_id, user_id, joined_at)
			 VALUES ($1, $2, NOW())`,
			convID, uid)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	// Update member count
	_, err = tx.ExecContext(ctx,
		`UPDATE groups SET current_members = current_members + $1, updated_at = NOW()
		 WHERE id=$2`, len(userIDs), groupID)
	if err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

// RemoveMember removes a user from a group.
func (s *Service) RemoveMember(ctx context.Context, groupID string, removedBy, targetUserID int64) error {
	// Cannot remove the owner
	var ownerID int64
	s.db.QueryRowContext(ctx,
		"SELECT creator_user_id FROM groups WHERE id=$1", groupID).Scan(&ownerID)
	if targetUserID == ownerID {
		return ErrCannotRemoveOwner
	}

	// Verify removedBy has permission
	var role string
	err := s.db.QueryRowContext(ctx,
		"SELECT role FROM group_members WHERE group_id=$1 AND user_id=$2 AND left_at IS NULL",
		groupID, removedBy).Scan(&role)
	if err == sql.ErrNoRows {
		return ErrNotMember
	}
	if err != nil {
		return err
	}
	if role != "owner" && role != "admin" {
		return ErrNotAdmin
	}

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
	convID := fmt.Sprintf("conv_%s", groupID)
	tx.ExecContext(ctx,
		`DELETE FROM conversation_members WHERE conversation_id=$1 AND user_id=$2`,
		convID, targetUserID)

	// Update member count
	tx.ExecContext(ctx,
		`UPDATE groups SET current_members = current_members - 1, updated_at = NOW() WHERE id=$1`,
		groupID)

	return tx.Commit()
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
	var role string
	err := s.db.QueryRowContext(ctx,
		"SELECT role FROM group_members WHERE group_id=$1 AND user_id=$2 AND left_at IS NULL",
		groupID, userID).Scan(&role)
	if err == sql.ErrNoRows {
		return ErrNotMember
	}
	if err != nil {
		return err
	}
	if role != "owner" && role != "admin" {
		return ErrNotAdmin
	}
	return nil
}
