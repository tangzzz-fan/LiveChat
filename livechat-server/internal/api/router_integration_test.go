package api

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/auth"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/infra"
	syncsvc "github.com/tangzzz-fan/LiveChat/livechat-server/internal/sync"
)

func TestRegisterConflictReturns409(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc)

	phone := uniquePhone(t)
	deviceA := uniqueDeviceID(t, "ios-reg-a")
	deviceB := uniqueDeviceID(t, "ios-reg-b")

	body := map[string]any{
		"phone_e164":        phone,
		"verification_code": "123456",
		"device_id":         deviceA,
		"platform":          "ios",
	}
	rec := doJSONRequest(t, router, http.MethodPost, "/v1/auth/register", body, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected first register 201, got %d: %s", rec.Code, rec.Body.String())
	}

	t.Cleanup(func() {
		cleanupAPIUsers(t, db, []int64{userIDByPhone(t, db, phone)}, []string{deviceA, deviceB})
	})

	body["device_id"] = deviceB
	rec = doJSONRequest(t, router, http.MethodPost, "/v1/auth/register", body, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected duplicate register 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSendMessageEndpointPersistsOutboxDeduplicatesAndAdvancesSeq(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc)

	convID := uniqueConversationID(t, "send-seq")
	userA := uniqueUserID(t, 1)
	userB := uniqueUserID(t, 2)
	seedAPIDirectConversation(t, db, convID, []apiUserSeed{
		{userID: userA, displayName: "A"},
		{userID: userB, displayName: "B"},
	})
	t.Cleanup(func() { cleanupAPIConversation(t, db, convID, []int64{userA, userB}) })

	tokenA, err := authSvc.SignAccessToken(userA, "ios-a")
	if err != nil {
		t.Fatalf("SignAccessToken userA: %v", err)
	}

	resp1 := sendMessageRequestAndDecode(t, router, tokenA, map[string]any{
		"client_message_id": "m-1",
		"conversation_id":   convID,
		"message_type":      "text",
		"content":           `{"text":"hello 1"}`,
	})
	resp2 := sendMessageRequestAndDecode(t, router, tokenA, map[string]any{
		"client_message_id": "m-2",
		"conversation_id":   convID,
		"message_type":      "text",
		"content":           `{"text":"hello 2"}`,
	})
	resp3 := sendMessageRequestAndDecode(t, router, tokenA, map[string]any{
		"client_message_id": "m-3",
		"conversation_id":   convID,
		"message_type":      "text",
		"content":           `{"text":"hello 3"}`,
	})
	dup := sendMessageRequestAndDecode(t, router, tokenA, map[string]any{
		"client_message_id": "m-3",
		"conversation_id":   convID,
		"message_type":      "text",
		"content":           `{"text":"hello 3"}`,
	})

	if resp1.ConversationSeq != 1 || resp2.ConversationSeq != 2 || resp3.ConversationSeq != 3 {
		t.Fatalf("expected seqs 1,2,3 got %d,%d,%d", resp1.ConversationSeq, resp2.ConversationSeq, resp3.ConversationSeq)
	}
	if dup.IsDuplicate != true {
		t.Fatalf("expected duplicate response to set is_duplicate=true")
	}
	if dup.ServerMessageID != resp3.ServerMessageID || dup.ConversationSeq != resp3.ConversationSeq {
		t.Fatalf("expected duplicate to reuse original identity, got %+v want msg=%s seq=%d", dup, resp3.ServerMessageID, resp3.ConversationSeq)
	}

	assertMessageCount(t, db, convID, 3)
	assertOutboxCount(t, db, convID, 3)
	assertOutboxRowForMessage(t, db, resp1.ServerMessageID, int(resp1.ConversationSeq))
}

func TestSendMessageEndpointScopesClientMessageIDPerUser(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc)

	convID := uniqueConversationID(t, "send-scope")
	userA := uniqueUserID(t, 11)
	userB := uniqueUserID(t, 12)
	seedAPIDirectConversation(t, db, convID, []apiUserSeed{
		{userID: userA, displayName: "A"},
		{userID: userB, displayName: "B"},
	})
	t.Cleanup(func() { cleanupAPIConversation(t, db, convID, []int64{userA, userB}) })

	tokenA, err := authSvc.SignAccessToken(userA, "ios-a")
	if err != nil {
		t.Fatalf("SignAccessToken userA: %v", err)
	}
	tokenB, err := authSvc.SignAccessToken(userB, "ios-b")
	if err != nil {
		t.Fatalf("SignAccessToken userB: %v", err)
	}

	respA := sendMessageRequestAndDecode(t, router, tokenA, map[string]any{
		"client_message_id": "shared-client-id",
		"conversation_id":   convID,
		"content":           `{"text":"from A"}`,
	})
	respB := sendMessageRequestAndDecode(t, router, tokenB, map[string]any{
		"client_message_id": "shared-client-id",
		"conversation_id":   convID,
		"content":           `{"text":"from B"}`,
	})

	if respA.ServerMessageID == respB.ServerMessageID {
		t.Fatalf("expected different users to get different server_message_id values")
	}
	assertMessageCount(t, db, convID, 2)
	assertOutboxCount(t, db, convID, 2)
}

func TestSendMessageEndpointRejectsUnauthorizedBadRequestAndNonMember(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc)

	convID := uniqueConversationID(t, "send-errors")
	userA := uniqueUserID(t, 21)
	userB := uniqueUserID(t, 22)
	userC := uniqueUserID(t, 23)
	seedAPIDirectConversation(t, db, convID, []apiUserSeed{
		{userID: userA, displayName: "A"},
		{userID: userB, displayName: "B"},
	})
	ensureAPIUsers(t, db, []apiUserSeed{{userID: userC, displayName: "C"}})
	t.Cleanup(func() { cleanupAPIConversation(t, db, convID, []int64{userA, userB, userC}) })

	tokenA, err := authSvc.SignAccessToken(userA, "ios-a")
	if err != nil {
		t.Fatalf("SignAccessToken userA: %v", err)
	}
	tokenC, err := authSvc.SignAccessToken(userC, "ios-c")
	if err != nil {
		t.Fatalf("SignAccessToken userC: %v", err)
	}

	rec := doJSONRequest(t, router, http.MethodPost, "/v1/messages/send", map[string]any{
		"client_message_id": "m-unauth",
		"conversation_id":   convID,
		"content":           `{"text":"hello"}`,
	}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing jwt 401, got %d", rec.Code)
	}

	rec = doJSONRequest(t, router, http.MethodPost, "/v1/messages/send", map[string]any{
		"conversation_id": convID,
		"content":         `{"text":"missing client_message_id"}`,
	}, tokenA)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected missing fields 400, got %d", rec.Code)
	}

	rec = doJSONRequest(t, router, http.MethodPost, "/v1/messages/send", map[string]any{
		"client_message_id": "m-forbidden",
		"conversation_id":   convID,
		"content":           `{"text":"not a member"}`,
	}, tokenC)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected non-member 403, got %d", rec.Code)
	}
}

func TestListConversationsRequiresAuth(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc)

	rec := doJSONRequest(t, router, http.MethodGet, "/v1/conversations", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized conversations list 401, got %d", rec.Code)
	}
}

func TestGetSyncEventsEndpointReturnsEventsLatestSeqAndUpdatesCursor(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc)

	const deviceID = "ios-sync-http"
	userID := uniqueUserID(t, 31)
	ensureAPIUsers(t, db, []apiUserSeed{{userID: userID, displayName: "sync-user"}})
	t.Cleanup(func() { cleanupAPIUsers(t, db, []int64{userID}, []string{deviceID}) })

	token, err := authSvc.SignAccessToken(userID, deviceID)
	if err != nil {
		t.Fatalf("SignAccessToken: %v", err)
	}

	seeded := seedAPISyncEvents(t, db, userID, "conv-sync-http", 3)

	rec := doJSONRequest(t, router, http.MethodGet, "/v1/sync/events?cursor=0", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected sync events 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Events []struct {
			EventSeq  int64  `json:"event_seq"`
			EventType string `json:"event_type"`
			Payload   string `json:"payload"`
		} `json:"events"`
		HasMore        bool  `json:"has_more"`
		LatestEventSeq int64 `json:"latest_event_seq"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode sync events response: %v", err)
	}
	if len(resp.Events) != 3 {
		t.Fatalf("expected 3 sync events, got %d", len(resp.Events))
	}
	if resp.HasMore {
		t.Fatalf("expected has_more=false for full sync page")
	}
	if resp.LatestEventSeq != seeded[len(seeded)-1] {
		t.Fatalf("expected latest_event_seq %d, got %d", seeded[len(seeded)-1], resp.LatestEventSeq)
	}
	for i, event := range resp.Events {
		if event.EventSeq != seeded[i] {
			t.Fatalf("expected event_seq[%d]=%d, got %d", i, seeded[i], event.EventSeq)
		}
		if event.EventType != "message_created" {
			t.Fatalf("expected event_type message_created, got %s", event.EventType)
		}
	}

	assertSyncCursor(t, db, userID, deviceID, seeded[len(seeded)-1])
}

func TestGetSyncEventsEndpointPaginatesAndCursorOnlyMovesForward(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc)

	const deviceID = "ios-sync-paging"
	userID := uniqueUserID(t, 41)
	ensureAPIUsers(t, db, []apiUserSeed{{userID: userID, displayName: "sync-user"}})
	t.Cleanup(func() { cleanupAPIUsers(t, db, []int64{userID}, []string{deviceID}) })

	token, err := authSvc.SignAccessToken(userID, deviceID)
	if err != nil {
		t.Fatalf("SignAccessToken: %v", err)
	}

	seeded := seedAPISyncEvents(t, db, userID, "conv-sync-page", 3)

	rec := doJSONRequest(t, router, http.MethodGet, "/v1/sync/events?cursor=0&limit=2", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected first sync page 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var page1 struct {
		Events []struct {
			EventSeq int64 `json:"event_seq"`
		} `json:"events"`
		HasMore        bool  `json:"has_more"`
		LatestEventSeq int64 `json:"latest_event_seq"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&page1); err != nil {
		t.Fatalf("decode first sync page: %v", err)
	}
	if len(page1.Events) != 2 {
		t.Fatalf("expected first sync page size 2, got %d", len(page1.Events))
	}
	if !page1.HasMore {
		t.Fatalf("expected first page has_more=true")
	}
	if page1.Events[0].EventSeq != seeded[0] || page1.Events[1].EventSeq != seeded[1] {
		t.Fatalf("unexpected first page event seqs: %+v", page1.Events)
	}
	if page1.LatestEventSeq != seeded[2] {
		t.Fatalf("expected latest_event_seq %d, got %d", seeded[2], page1.LatestEventSeq)
	}
	assertSyncCursor(t, db, userID, deviceID, seeded[1])

	rec = doJSONRequest(t, router, http.MethodGet, fmt.Sprintf("/v1/sync/events?cursor=%d&limit=2", seeded[1]), nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected second sync page 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var page2 struct {
		Events []struct {
			EventSeq int64 `json:"event_seq"`
		} `json:"events"`
		HasMore        bool  `json:"has_more"`
		LatestEventSeq int64 `json:"latest_event_seq"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&page2); err != nil {
		t.Fatalf("decode second sync page: %v", err)
	}
	if len(page2.Events) != 1 {
		t.Fatalf("expected second sync page size 1, got %d", len(page2.Events))
	}
	if page2.HasMore {
		t.Fatalf("expected second page has_more=false")
	}
	if page2.Events[0].EventSeq != seeded[2] {
		t.Fatalf("expected second page final event seq %d, got %d", seeded[2], page2.Events[0].EventSeq)
	}
	assertSyncCursor(t, db, userID, deviceID, seeded[2])

	rec = doJSONRequest(t, router, http.MethodGet, "/v1/sync/events?cursor=0&limit=1", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected replayed old cursor request 200, got %d: %s", rec.Code, rec.Body.String())
	}
	assertSyncCursor(t, db, userID, deviceID, seeded[2])
}

func openAPITestDB(t *testing.T) *sql.DB {
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

type apiUserSeed struct {
	userID      int64
	displayName string
}

type sendMessageResponse struct {
	ServerMessageID string `json:"server_message_id"`
	ConversationSeq int64  `json:"conversation_seq"`
	IsDuplicate     bool   `json:"is_duplicate"`
}

func sendMessageRequestAndDecode(t *testing.T, handler http.Handler, token string, body map[string]any) sendMessageResponse {
	t.Helper()

	rec := doJSONRequest(t, handler, http.MethodPost, "/v1/messages/send", body, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected send message 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp sendMessageResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode send message response: %v", err)
	}
	return resp
}

func doJSONRequest(t *testing.T, handler http.Handler, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()

	var payload []byte
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal body: %v", err)
		}
		payload = raw
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func seedAPIDirectConversation(t *testing.T, db *sql.DB, conversationID string, users []apiUserSeed) {
	t.Helper()
	ensureAPIUsers(t, db, users)
	mustExecAPI(t, db, `INSERT INTO conversations (id, type) VALUES ($1, 'direct')`, conversationID)
	for _, user := range users {
		mustExecAPI(t, db,
			`INSERT INTO conversation_members (conversation_id, user_id) VALUES ($1, $2)`,
			conversationID, user.userID,
		)
	}
}

func ensureAPIUsers(t *testing.T, db *sql.DB, users []apiUserSeed) {
	t.Helper()
	for _, user := range users {
		mustExecAPI(t, db,
			`INSERT INTO users (id, phone_e164, display_name) VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING`,
			user.userID, phoneForAPIUser(user.userID), user.displayName,
		)
	}
}

func cleanupAPIConversation(t *testing.T, db *sql.DB, conversationID string, userIDs []int64) {
	t.Helper()
	mustExecAPI(t, db, `DELETE FROM conversation_summaries WHERE conversation_id=$1`, conversationID)
	mustExecAPI(t, db, `DELETE FROM sync_cursors WHERE user_id = ANY($1)`, pqInt64Array(userIDs))
	mustExecAPI(t, db, `DELETE FROM sync_events WHERE conversation_id=$1 OR user_id = ANY($2)`, conversationID, pqInt64Array(userIDs))
	mustExecAPI(t, db, `DELETE FROM outbox_events WHERE aggregate_id LIKE $1`, "msg_"+conversationID+"_%")
	mustExecAPI(t, db, `DELETE FROM messages WHERE conversation_id=$1`, conversationID)
	mustExecAPI(t, db, `DELETE FROM conversation_members WHERE conversation_id=$1`, conversationID)
	mustExecAPI(t, db, `DELETE FROM conversations WHERE id=$1`, conversationID)
	cleanupAPIUsers(t, db, userIDs, nil)
	mustExecAPIIgnoringError(t, db, fmt.Sprintf(`DROP SEQUENCE IF EXISTS conversation_seq_%s`, strings.ReplaceAll(conversationID, "-", "_")))
}

func cleanupAPIUsers(t *testing.T, db *sql.DB, userIDs []int64, deviceIDs []string) {
	t.Helper()
	if len(deviceIDs) > 0 {
		mustExecAPI(t, db, `DELETE FROM devices WHERE id = ANY($1)`, pqStringArray(deviceIDs))
	}
	if len(userIDs) == 0 {
		return
	}
	mustExecAPI(t, db, `DELETE FROM conversation_summaries WHERE user_id = ANY($1)`, pqInt64Array(userIDs))
	mustExecAPI(t, db, `DELETE FROM sync_cursors WHERE user_id = ANY($1)`, pqInt64Array(userIDs))
	mustExecAPI(t, db, `DELETE FROM sync_events WHERE user_id = ANY($1)`, pqInt64Array(userIDs))
	mustExecAPI(t, db, `DELETE FROM devices WHERE user_id = ANY($1)`, pqInt64Array(userIDs))
	mustExecAPI(t, db, `DELETE FROM conversation_members WHERE user_id = ANY($1)`, pqInt64Array(userIDs))
	mustExecAPI(t, db, `DELETE FROM users WHERE id = ANY($1)`, pqInt64Array(userIDs))
}

func assertMessageCount(t *testing.T, db *sql.DB, conversationID string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM messages WHERE conversation_id=$1`,
		conversationID,
	).Scan(&got); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if got != want {
		t.Fatalf("conversation %s: want %d messages, got %d", conversationID, want, got)
	}
}

func assertOutboxCount(t *testing.T, db *sql.DB, conversationID string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM outbox_events WHERE aggregate_id LIKE $1`,
		"msg_"+conversationID+"_%",
	).Scan(&got); err != nil {
		t.Fatalf("count outbox events: %v", err)
	}
	if got != want {
		t.Fatalf("conversation %s: want %d outbox events, got %d", conversationID, want, got)
	}
}

func assertOutboxRowForMessage(t *testing.T, db *sql.DB, serverMessageID string, wantSeq int) {
	t.Helper()
	var aggregateType string
	var eventType string
	var status string
	var payloadRaw []byte
	if err := db.QueryRowContext(context.Background(),
		`SELECT aggregate_type, event_type, status, payload FROM outbox_events WHERE aggregate_id=$1`,
		serverMessageID,
	).Scan(&aggregateType, &eventType, &status, &payloadRaw); err != nil {
		t.Fatalf("load outbox row: %v", err)
	}
	if aggregateType != "message" || eventType != "message_created" || status != "pending" {
		t.Fatalf("unexpected outbox row: aggregate_type=%s event_type=%s status=%s", aggregateType, eventType, status)
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadRaw, &payload); err != nil {
		t.Fatalf("unmarshal outbox payload: %v", err)
	}
	if payload["server_message_id"] != serverMessageID {
		t.Fatalf("unexpected outbox server_message_id: %+v", payload["server_message_id"])
	}
	gotSeq, ok := payload["conversation_seq"].(float64)
	if !ok || int(gotSeq) != wantSeq {
		t.Fatalf("unexpected outbox conversation_seq: %+v", payload["conversation_seq"])
	}
}

func userIDByPhone(t *testing.T, db *sql.DB, phone string) int64 {
	t.Helper()
	var userID int64
	if err := db.QueryRowContext(context.Background(),
		`SELECT id FROM users WHERE phone_e164=$1`,
		phone,
	).Scan(&userID); err != nil {
		t.Fatalf("lookup user by phone %s: %v", phone, err)
	}
	return userID
}

func mustExecAPI(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("ExecContext %q: %v", query, err)
	}
}

func mustExecAPIIgnoringError(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	_, _ = db.ExecContext(context.Background(), query, args...)
}

func uniqueConversationID(t *testing.T, prefix string) string {
	t.Helper()
	return fmt.Sprintf("test-%s-%d", prefix, time.Now().UnixNano())
}

func uniqueDeviceID(t *testing.T, prefix string) string {
	t.Helper()
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func uniquePhone(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("+1555%d", time.Now().UnixNano()%1_000_000_000)
}

func uniqueUserID(t *testing.T, salt int64) int64 {
	t.Helper()
	return time.Now().UnixNano()%1_000_000_000 + salt
}

func phoneForAPIUser(userID int64) string {
	return fmt.Sprintf("+1666%06d", userID%1_000_000)
}

func seedAPISyncEvents(t *testing.T, db *sql.DB, userID int64, conversationID string, count int) []int64 {
	t.Helper()
	svc := syncsvc.NewService(db)
	seqs := make([]int64, 0, count)
	for i := 1; i <= count; i++ {
		payload := []byte(fmt.Sprintf(`{"n":%d}`, i))
		if err := svc.AppendEventWithConv(context.Background(), userID, conversationID, "message_created", payload); err != nil {
			t.Fatalf("AppendEventWithConv #%d: %v", i, err)
		}
		var seq int64
		if err := db.QueryRowContext(context.Background(),
			`SELECT COALESCE(MAX(event_seq), 0) FROM sync_events WHERE user_id=$1`,
			userID,
		).Scan(&seq); err != nil {
			t.Fatalf("query latest sync event seq: %v", err)
		}
		seqs = append(seqs, seq)
	}
	return seqs
}

func assertSyncCursor(t *testing.T, db *sql.DB, userID int64, deviceID string, want int64) {
	t.Helper()
	var got int64
	if err := db.QueryRowContext(context.Background(),
		`SELECT last_event_seq FROM sync_cursors WHERE user_id=$1 AND device_id=$2`,
		userID, deviceID,
	).Scan(&got); err != nil {
		t.Fatalf("query sync cursor: %v", err)
	}
	if got != want {
		t.Fatalf("user %d device %s: want cursor %d, got %d", userID, deviceID, want, got)
	}
}

type pqInt64Array []int64

func (a pqInt64Array) Value() (driver.Value, error) {
	if len(a) == 0 {
		return "{}", nil
	}
	parts := make([]string, 0, len(a))
	for _, n := range a {
		parts = append(parts, fmt.Sprintf("%d", n))
	}
	return "{" + strings.Join(parts, ",") + "}", nil
}

type pqStringArray []string

func (a pqStringArray) Value() (driver.Value, error) {
	if len(a) == 0 {
		return "{}", nil
	}
	quoted := make([]string, 0, len(a))
	for _, s := range a {
		quoted = append(quoted, `"`+strings.ReplaceAll(s, `"`, `\"`)+`"`)
	}
	return "{" + strings.Join(quoted, ",") + "}", nil
}
