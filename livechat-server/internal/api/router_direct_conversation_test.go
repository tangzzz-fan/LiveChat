package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/auth"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/conversations"
)

type directConvResponse struct {
	ConversationID string `json:"conversation_id"`
	Type           string `json:"type"`
	PeerUserID     int64  `json:"peer_user_id"`
	Created        bool   `json:"created"`
}

func TestCreateDirectConversationIsIdempotentAndOrderIndependent(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	userA := uniqueUserID(t, 401)
	userB := uniqueUserID(t, 402)
	deviceA := uniqueDeviceID(t, "ios-dm-a")
	deviceB := uniqueDeviceID(t, "ios-dm-b")
	ensureAPIUsers(t, db, []apiUserSeed{
		{userID: userA, displayName: "DM A"},
		{userID: userB, displayName: "DM B"},
	})
	seedAPIDevice(t, db, userA, deviceA, "ios")
	seedAPIDevice(t, db, userB, deviceB, "ios")

	convID := conversations.DirectConversationID(userA, userB)
	t.Cleanup(func() {
		cleanupAPIConversation(t, db, convID, []int64{userA, userB})
	})

	tokenA, err := authSvc.SignAccessToken(userA, deviceA, 1)
	if err != nil {
		t.Fatalf("SignAccessToken A: %v", err)
	}
	tokenB, err := authSvc.SignAccessToken(userB, deviceB, 1)
	if err != nil {
		t.Fatalf("SignAccessToken B: %v", err)
	}

	first := postDirectConversation(t, router, tokenA, userB)
	if !first.Created {
		t.Fatalf("expected first call to report created=true, got %+v", first)
	}
	if first.Type != "direct" {
		t.Fatalf("expected type=direct, got %q", first.Type)
	}

	// Same caller again: idempotent replay.
	second := postDirectConversation(t, router, tokenA, userB)
	if second.ConversationID != first.ConversationID {
		t.Fatalf("expected same conversation on replay, got %s vs %s", second.ConversationID, first.ConversationID)
	}
	if second.Created {
		t.Fatalf("expected created=false on replay, got %+v", second)
	}

	// Peer opens the same chat from the other direction: must land on the
	// same conversation, not create a second one.
	fromPeer := postDirectConversation(t, router, tokenB, userA)
	if fromPeer.ConversationID != first.ConversationID {
		t.Fatalf("expected order-independent id, got %s vs %s", fromPeer.ConversationID, first.ConversationID)
	}
	if fromPeer.Created {
		t.Fatalf("expected created=false when peer opens existing chat, got %+v", fromPeer)
	}

	assertDirectConversationRows(t, db, first.ConversationID, 2)
}

func TestDirectConversationAllowsBothMembersToSend(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	userA := uniqueUserID(t, 411)
	userB := uniqueUserID(t, 412)
	userC := uniqueUserID(t, 413)
	deviceA := uniqueDeviceID(t, "ios-dmsend-a")
	deviceB := uniqueDeviceID(t, "ios-dmsend-b")
	deviceC := uniqueDeviceID(t, "ios-dmsend-c")
	ensureAPIUsers(t, db, []apiUserSeed{
		{userID: userA, displayName: "Sender A"},
		{userID: userB, displayName: "Sender B"},
		{userID: userC, displayName: "Outsider C"},
	})
	seedAPIDevice(t, db, userA, deviceA, "ios")
	seedAPIDevice(t, db, userB, deviceB, "ios")
	seedAPIDevice(t, db, userC, deviceC, "ios")

	convID := conversations.DirectConversationID(userA, userB)
	t.Cleanup(func() {
		cleanupAPIConversation(t, db, convID, []int64{userA, userB, userC})
	})

	tokenA := mustSignToken(t, authSvc, userA, deviceA)
	tokenB := mustSignToken(t, authSvc, userB, deviceB)
	tokenC := mustSignToken(t, authSvc, userC, deviceC)

	created := postDirectConversation(t, router, tokenA, userB)

	for i, token := range []string{tokenA, tokenB} {
		rec := doJSONRequest(t, router, http.MethodPost, "/v1/messages/send", map[string]any{
			"client_message_id": "dm-msg-" + string(rune('a'+i)),
			"conversation_id":   created.ConversationID,
			"message_type":      "text",
			"content":           `{"text":"hi"}`,
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("member %d send: expected 200, got %d: %s", i, rec.Code, rec.Body.String())
		}
	}

	// A third user is not a member and must be rejected.
	rec := doJSONRequest(t, router, http.MethodPost, "/v1/messages/send", map[string]any{
		"client_message_id": "dm-msg-outsider",
		"conversation_id":   created.ConversationID,
		"message_type":      "text",
		"content":           `{"text":"let me in"}`,
	}, tokenC)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("outsider send: expected 403, got %d: %s", rec.Code, rec.Body.String())
	}

	assertMessageCount(t, db, created.ConversationID, 2)
}

func TestCreateDirectConversationRejectsBadInput(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	userA := uniqueUserID(t, 421)
	deviceA := uniqueDeviceID(t, "ios-dmbad-a")
	ensureAPIUsers(t, db, []apiUserSeed{{userID: userA, displayName: "Solo"}})
	seedAPIDevice(t, db, userA, deviceA, "ios")
	t.Cleanup(func() { cleanupAPIUsers(t, db, []int64{userA}, []string{deviceA}) })

	tokenA := mustSignToken(t, authSvc, userA, deviceA)

	cases := []struct {
		name     string
		body     map[string]any
		token    string
		wantCode int
	}{
		{"unauthorized", map[string]any{"peer_user_id": userA + 1}, "", http.StatusUnauthorized},
		{"missing peer", map[string]any{}, tokenA, http.StatusBadRequest},
		{"self conversation", map[string]any{"peer_user_id": userA}, tokenA, http.StatusBadRequest},
		{"unknown peer", map[string]any{"peer_user_id": int64(999999999)}, tokenA, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doJSONRequest(t, router, http.MethodPost, "/v1/conversations/direct", tc.body, tc.token)
			if rec.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d: %s", tc.wantCode, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestDirectConversationIDIsStableAndOrderIndependent(t *testing.T) {
	if got, want := conversations.DirectConversationID(7, 3), "conv_dm_3_7"; got != want {
		t.Fatalf("DirectConversationID(7,3) = %q, want %q", got, want)
	}
	if a, b := conversations.DirectConversationID(3, 7), conversations.DirectConversationID(7, 3); a != b {
		t.Fatalf("expected order-independent id, got %q vs %q", a, b)
	}
}

// ── helpers ─────────────────────────────────────────

func mustSignToken(t *testing.T, authSvc *auth.Service, userID int64, deviceID string) string {
	t.Helper()
	token, err := authSvc.SignAccessToken(userID, deviceID, 1)
	if err != nil {
		t.Fatalf("SignAccessToken(%d): %v", userID, err)
	}
	return token
}

func postDirectConversation(t *testing.T, handler http.Handler, token string, peerUserID int64) directConvResponse {
	t.Helper()
	rec := doJSONRequest(t, handler, http.MethodPost, "/v1/conversations/direct", map[string]any{
		"peer_user_id": peerUserID,
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("create direct conversation: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp directConvResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode direct conversation response: %v", err)
	}
	return resp
}

func assertDirectConversationRows(t *testing.T, db *sql.DB, conversationID string, wantMembers int) {
	t.Helper()
	var convType string
	if err := db.QueryRowContext(context.Background(),
		`SELECT type FROM conversations WHERE id = $1`, conversationID,
	).Scan(&convType); err != nil {
		t.Fatalf("load conversation: %v", err)
	}
	if convType != "direct" {
		t.Fatalf("expected type=direct, got %q", convType)
	}

	var members int
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM conversation_members WHERE conversation_id = $1`, conversationID,
	).Scan(&members); err != nil {
		t.Fatalf("count members: %v", err)
	}
	if members != wantMembers {
		t.Fatalf("expected %d members, got %d", wantMembers, members)
	}
}
