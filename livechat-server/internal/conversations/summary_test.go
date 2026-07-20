package conversations

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/infra"
)

func TestUpdateOnNewMessageAccumulatesUnreadCounts(t *testing.T) {
	db := openConversationTestDB(t)
	ctx := context.Background()

	const (
		userA  = int64(92001)
		userB  = int64(92002)
		convID = "test-summary-unread"
	)
	cleanupConversationFixture(t, db, []string{convID}, []int64{userA, userB})
	t.Cleanup(func() {
		cleanupConversationFixture(t, db, []string{convID}, []int64{userA, userB})
	})
	seedConversationFixture(t, db, conversationSeed{
		id:      convID,
		members: []memberSeed{{userID: userA, displayName: "A"}, {userID: userB, displayName: "B"}},
	})

	svc := NewService(db)
	for i := 0; i < 3; i++ {
		if err := svc.UpdateOnNewMessage(ctx, convID, userA, "hello"); err != nil {
			t.Fatalf("UpdateOnNewMessage #%d: %v", i+1, err)
		}
	}

	assertUnreadCount(t, db, userA, convID, 0)
	assertUnreadCount(t, db, userB, convID, 3)
}

func TestListReturnsMembersAndSupportsPagination(t *testing.T) {
	db := openConversationTestDB(t)
	ctx := context.Background()

	const (
		userA = int64(92011)
		userB = int64(92012)
		userC = int64(92013)
	)
	convIDs := []string{"test-summary-list-1", "test-summary-list-2"}
	cleanupConversationFixture(t, db, convIDs, []int64{userA, userB, userC})
	t.Cleanup(func() {
		cleanupConversationFixture(t, db, convIDs, []int64{userA, userB, userC})
	})
	seedConversationFixture(t, db,
		conversationSeed{
			id: "test-summary-list-1",
			members: []memberSeed{
				{userID: userA, displayName: "A"},
				{userID: userB, displayName: "B"},
			},
		},
		conversationSeed{
			id: "test-summary-list-2",
			members: []memberSeed{
				{userID: userA, displayName: "A"},
				{userID: userC, displayName: "C"},
			},
		},
	)

	mustExecConversation(t, db,
		`INSERT INTO conversation_summaries (
			user_id, conversation_id, last_message_preview, last_message_at, unread_count, updated_at
		) VALUES
		($1, $2, 'newest', NOW(), 2, NOW()),
		($1, $3, 'older', NOW() - INTERVAL '1 minute', 1, NOW())`,
		userA, convIDs[0], convIDs[1],
	)

	svc := NewService(db)
	page1, err := svc.List(ctx, userA, 1, 0)
	if err != nil {
		t.Fatalf("List page1: %v", err)
	}
	if len(page1) != 1 {
		t.Fatalf("expected page1 size 1, got %d", len(page1))
	}
	if page1[0].ConversationID != convIDs[0] {
		t.Fatalf("expected newest conversation first, got %s", page1[0].ConversationID)
	}
	if len(page1[0].Members) != 2 {
		t.Fatalf("expected members to be populated, got %d", len(page1[0].Members))
	}
	if page1[0].Members[0].UserID != userA || page1[0].Members[1].UserID != userB {
		t.Fatalf("unexpected member list for first page: %+v", page1[0].Members)
	}

	page2, err := svc.List(ctx, userA, 1, 1)
	if err != nil {
		t.Fatalf("List page2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("expected page2 size 1, got %d", len(page2))
	}
	if page2[0].ConversationID != convIDs[1] {
		t.Fatalf("expected second conversation on page2, got %s", page2[0].ConversationID)
	}
}

func TestListReturnsEmptySliceForUserWithoutConversations(t *testing.T) {
	db := openConversationTestDB(t)
	ctx := context.Background()

	const userID = int64(92021)
	cleanupConversationFixture(t, db, nil, []int64{userID})
	t.Cleanup(func() {
		cleanupConversationFixture(t, db, nil, []int64{userID})
	})
	mustExecConversation(t, db, `INSERT INTO users (id, phone_e164, display_name) VALUES ($1, $2, $3)`, userID, "+155592021", "lonely")

	svc := NewService(db)
	got, err := svc.List(ctx, userID, 50, 0)
	if err != nil {
		t.Fatalf("List empty: %v", err)
	}
	if got == nil {
		t.Fatalf("expected empty slice, got nil")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty slice length 0, got %d", len(got))
	}
}

type conversationSeed struct {
	id      string
	members []memberSeed
}

type memberSeed struct {
	userID      int64
	displayName string
}

func openConversationTestDB(t *testing.T) *sql.DB {
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

func seedConversationFixture(t *testing.T, db *sql.DB, conversations ...conversationSeed) {
	t.Helper()

	seenUsers := make(map[int64]memberSeed)
	for _, conv := range conversations {
		mustExecConversation(t, db, `INSERT INTO conversations (id, type) VALUES ($1, 'direct')`, conv.id)
		for _, member := range conv.members {
			if _, ok := seenUsers[member.userID]; !ok {
				seenUsers[member.userID] = member
				mustExecConversation(t, db,
					`INSERT INTO users (id, phone_e164, display_name) VALUES ($1, $2, $3)`,
					member.userID, phoneForUser(member.userID), member.displayName,
				)
			}
			mustExecConversation(t, db,
				`INSERT INTO conversation_members (conversation_id, user_id) VALUES ($1, $2)`,
				conv.id, member.userID,
			)
		}
	}
}

func cleanupConversationFixture(t *testing.T, db *sql.DB, convIDs []string, userIDs []int64) {
	t.Helper()
	for _, convID := range convIDs {
		mustExecConversation(t, db, `DELETE FROM conversation_summaries WHERE conversation_id=$1`, convID)
		mustExecConversation(t, db, `DELETE FROM conversation_members WHERE conversation_id=$1`, convID)
		mustExecConversation(t, db, `DELETE FROM conversations WHERE id=$1`, convID)
	}
	for _, userID := range userIDs {
		mustExecConversation(t, db, `DELETE FROM conversation_summaries WHERE user_id=$1`, userID)
		mustExecConversation(t, db, `DELETE FROM conversation_members WHERE user_id=$1`, userID)
		mustExecConversation(t, db, `DELETE FROM users WHERE id=$1`, userID)
	}
}

func assertUnreadCount(t *testing.T, db *sql.DB, userID int64, conversationID string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRowContext(context.Background(),
		`SELECT unread_count FROM conversation_summaries WHERE user_id=$1 AND conversation_id=$2`,
		userID, conversationID,
	).Scan(&got); err != nil {
		t.Fatalf("query unread_count: %v", err)
	}
	if got != want {
		t.Fatalf("user %d conversation %s: want unread=%d, got %d", userID, conversationID, want, got)
	}
}

func mustExecConversation(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("ExecContext %q: %v", query, err)
	}
}

func phoneForUser(userID int64) string {
	return fmt.Sprintf("+1555%06d", userID%1000000)
}
