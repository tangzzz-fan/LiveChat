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
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

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
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	convID := uniqueConversationID(t, "send-seq")
	userA := uniqueUserID(t, 1)
	userB := uniqueUserID(t, 2)
	seedAPIDirectConversation(t, db, convID, []apiUserSeed{
		{userID: userA, displayName: "A"},
		{userID: userB, displayName: "B"},
	})
	t.Cleanup(func() { cleanupAPIConversation(t, db, convID, []int64{userA, userB}) })

	seedAPIDevice(t, db, userA, "ios-a", "ios")
	tokenA, err := authSvc.SignAccessToken(userA, "ios-a", 1)
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
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	convID := uniqueConversationID(t, "send-scope")
	userA := uniqueUserID(t, 11)
	userB := uniqueUserID(t, 12)
	seedAPIDirectConversation(t, db, convID, []apiUserSeed{
		{userID: userA, displayName: "A"},
		{userID: userB, displayName: "B"},
	})
	t.Cleanup(func() { cleanupAPIConversation(t, db, convID, []int64{userA, userB}) })

	seedAPIDevice(t, db, userA, "ios-a", "ios")
	tokenA, err := authSvc.SignAccessToken(userA, "ios-a", 1)
	if err != nil {
		t.Fatalf("SignAccessToken userA: %v", err)
	}
	seedAPIDevice(t, db, userB, "ios-b", "ios")
	tokenB, err := authSvc.SignAccessToken(userB, "ios-b", 1)
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
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

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

	seedAPIDevice(t, db, userA, "ios-a", "ios")
	tokenA, err := authSvc.SignAccessToken(userA, "ios-a", 1)
	if err != nil {
		t.Fatalf("SignAccessToken userA: %v", err)
	}
	seedAPIDevice(t, db, userC, "ios-c", "ios")
	tokenC, err := authSvc.SignAccessToken(userC, "ios-c", 1)
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
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	rec := doJSONRequest(t, router, http.MethodGet, "/v1/conversations", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized conversations list 401, got %d", rec.Code)
	}
}

func TestLoginReturnsJWTAndAllowsProtectedEndpoint(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	phone := uniquePhone(t)
	deviceID := uniqueDeviceID(t, "ios-login")

	register := doJSONRequest(t, router, http.MethodPost, "/v1/auth/register", map[string]any{
		"phone_e164":        phone,
		"verification_code": "123456",
		"device_id":         "bootstrap-" + deviceID,
		"platform":          "ios",
	}, "")
	if register.Code != http.StatusCreated {
		t.Fatalf("expected register 201, got %d: %s", register.Code, register.Body.String())
	}

	userID := userIDByPhone(t, db, phone)
	t.Cleanup(func() {
		cleanupAPIUsers(t, db, []int64{userID}, []string{"bootstrap-" + deviceID, deviceID})
	})

	login := doJSONRequest(t, router, http.MethodPost, "/v1/auth/login", map[string]any{
		"phone_e164":        phone,
		"verification_code": "654321",
		"device_id":         deviceID,
		"platform":          "ios",
	}, "")
	if login.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d: %s", login.Code, login.Body.String())
	}

	var loginResp struct {
		AccessToken string `json:"access_token"`
		UserID      int64  `json:"user_id"`
	}
	if err := json.NewDecoder(login.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if loginResp.AccessToken == "" {
		t.Fatalf("expected login access_token")
	}
	if loginResp.UserID != userID {
		t.Fatalf("expected login user_id %d, got %d", userID, loginResp.UserID)
	}

	protected := doJSONRequest(t, router, http.MethodGet, "/v1/devices", nil, loginResp.AccessToken)
	if protected.Code != http.StatusOK {
		t.Fatalf("expected protected devices endpoint 200, got %d: %s", protected.Code, protected.Body.String())
	}
}

func TestHealthReturnsPostgresAndRedisStatus(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	rec := doJSONRequest(t, router, http.MethodGet, "/health", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected health 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Status  string            `json:"status"`
		Details map[string]string `json:"details"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected health status ok, got %s", resp.Status)
	}
	if resp.Details["postgres"] != "ok" {
		t.Fatalf("expected postgres health ok, got %s", resp.Details["postgres"])
	}
	if resp.Details["redis"] != "ok" {
		t.Fatalf("expected redis health ok, got %s", resp.Details["redis"])
	}
}

func TestGetSyncEventsEndpointReturnsEventsLatestSeqAndUpdatesCursor(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	const deviceID = "ios-sync-http"
	userID := uniqueUserID(t, 31)
	ensureAPIUsers(t, db, []apiUserSeed{{userID: userID, displayName: "sync-user"}})
	seedAPIDevice(t, db, userID, deviceID, "ios")
	t.Cleanup(func() { cleanupAPIUsers(t, db, []int64{userID}, []string{deviceID}) })

	token, err := authSvc.SignAccessToken(userID, deviceID, 1)
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
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	const deviceID = "ios-sync-paging"
	userID := uniqueUserID(t, 41)
	ensureAPIUsers(t, db, []apiUserSeed{{userID: userID, displayName: "sync-user"}})
	seedAPIDevice(t, db, userID, deviceID, "ios")
	t.Cleanup(func() { cleanupAPIUsers(t, db, []int64{userID}, []string{deviceID}) })

	token, err := authSvc.SignAccessToken(userID, deviceID, 1)
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

func TestGetSyncEventsEndpointSupports150EventPagination(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	const deviceID = "ios-sync-150"
	userID := uniqueUserID(t, 43)
	ensureAPIUsers(t, db, []apiUserSeed{{userID: userID, displayName: "sync-user"}})
	seedAPIDevice(t, db, userID, deviceID, "ios")
	t.Cleanup(func() { cleanupAPIUsers(t, db, []int64{userID}, []string{deviceID}) })

	token, err := authSvc.SignAccessToken(userID, deviceID, 1)
	if err != nil {
		t.Fatalf("SignAccessToken: %v", err)
	}

	seeded := seedAPISyncEvents(t, db, userID, "conv-sync-150", 150)

	rec := doJSONRequest(t, router, http.MethodGet, "/v1/sync/events?cursor=0&limit=100", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected first 150-page response 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var page1 struct {
		Events []struct {
			EventSeq int64 `json:"event_seq"`
		} `json:"events"`
		HasMore bool `json:"has_more"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&page1); err != nil {
		t.Fatalf("decode first 150-page response: %v", err)
	}
	if len(page1.Events) != 100 {
		t.Fatalf("expected first page size 100, got %d", len(page1.Events))
	}
	if !page1.HasMore {
		t.Fatalf("expected first page has_more=true for 150 events")
	}

	lastSeqPage1 := page1.Events[len(page1.Events)-1].EventSeq
	rec = doJSONRequest(t, router, http.MethodGet, fmt.Sprintf("/v1/sync/events?cursor=%d&limit=100", lastSeqPage1), nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected second 150-page response 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var page2 struct {
		Events []struct {
			EventSeq int64 `json:"event_seq"`
		} `json:"events"`
		HasMore bool `json:"has_more"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&page2); err != nil {
		t.Fatalf("decode second 150-page response: %v", err)
	}
	if len(page2.Events) != 50 {
		t.Fatalf("expected second page size 50, got %d", len(page2.Events))
	}
	if page2.HasMore {
		t.Fatalf("expected second page has_more=false for 150 events")
	}
	if page2.Events[len(page2.Events)-1].EventSeq != seeded[len(seeded)-1] {
		t.Fatalf("expected final event seq %d, got %d", seeded[len(seeded)-1], page2.Events[len(page2.Events)-1].EventSeq)
	}
	assertSyncCursor(t, db, userID, deviceID, seeded[len(seeded)-1])
}

func TestGetConversationMessagesSupportsGapRecoveryAndOrdering(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	convID := uniqueConversationID(t, "gap-recovery")
	userA := uniqueUserID(t, 51)
	userB := uniqueUserID(t, 52)
	seedAPIDirectConversation(t, db, convID, []apiUserSeed{
		{userID: userA, displayName: "A"},
		{userID: userB, displayName: "B"},
	})
	t.Cleanup(func() { cleanupAPIConversation(t, db, convID, []int64{userA, userB}) })
	seedConversationMessages(t, db, convID, userA, "ios-a", []int64{1, 2, 3})

	seedAPIDevice(t, db, userA, "ios-a", "ios")
	tokenA, err := authSvc.SignAccessToken(userA, "ios-a", 1)
	if err != nil {
		t.Fatalf("SignAccessToken: %v", err)
	}

	rec := doJSONRequest(t, router, http.MethodGet, "/v1/conversations/"+convID+"/messages?from_seq=2&limit=2", nil, tokenA)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected messages pull 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Messages []struct {
			ConversationSeq int64  `json:"conversation_seq"`
			ServerMessageID string `json:"server_message_id"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode messages response: %v", err)
	}
	if len(resp.Messages) != 2 {
		t.Fatalf("expected 2 messages from gap recovery pull, got %d", len(resp.Messages))
	}
	if resp.Messages[0].ConversationSeq != 2 || resp.Messages[1].ConversationSeq != 3 {
		t.Fatalf("expected ordered seq 2,3 got %+v", resp.Messages)
	}
}

func TestGetConversationMessagesRejectsNonMemberAndBadCursor(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	convID := uniqueConversationID(t, "gap-errors")
	userA := uniqueUserID(t, 61)
	userB := uniqueUserID(t, 62)
	userC := uniqueUserID(t, 63)
	seedAPIDirectConversation(t, db, convID, []apiUserSeed{
		{userID: userA, displayName: "A"},
		{userID: userB, displayName: "B"},
	})
	ensureAPIUsers(t, db, []apiUserSeed{{userID: userC, displayName: "C"}})
	t.Cleanup(func() { cleanupAPIConversation(t, db, convID, []int64{userA, userB, userC}) })

	seedAPIDevice(t, db, userA, "ios-a", "ios")
	tokenA, err := authSvc.SignAccessToken(userA, "ios-a", 1)
	if err != nil {
		t.Fatalf("SignAccessToken userA: %v", err)
	}
	seedAPIDevice(t, db, userC, "ios-c", "ios")
	tokenC, err := authSvc.SignAccessToken(userC, "ios-c", 1)
	if err != nil {
		t.Fatalf("SignAccessToken userC: %v", err)
	}

	rec := doJSONRequest(t, router, http.MethodGet, "/v1/conversations/"+convID+"/messages?from_seq=bad", nil, tokenA)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected bad from_seq 400, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = doJSONRequest(t, router, http.MethodGet, "/v1/conversations/"+convID+"/messages?from_seq=1", nil, tokenC)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected non-member messages pull 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ── Group membership tests ─────────────────────────────

func TestCreateGroupAtomicallyCreatesConversationAndOwner(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	userA := uniqueUserID(t, 71)
	deviceA := uniqueDeviceID(t, "ios-group")
	ensureAPIUsers(t, db, []apiUserSeed{{userID: userA, displayName: "Group Creator"}})
	seedAPIDevice(t, db, userA, deviceA, "ios")
	t.Cleanup(func() {
		cleanupAPIUsers(t, db, []int64{userA}, []string{deviceA})
	})

	tokenA, err := authSvc.SignAccessToken(userA, deviceA, 1)
	if err != nil {
		t.Fatalf("SignAccessToken: %v", err)
	}

	rec := doJSONRequest(t, router, http.MethodPost, "/v1/groups", map[string]any{
		"name":        "Test Group",
		"description": "A test group",
	}, tokenA)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected create group 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Group          map[string]any `json:"group"`
		ConversationID string         `json:"conversation_id"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Group["id"] == "" {
		t.Fatalf("expected group id")
	}
	if resp.ConversationID == "" {
		t.Fatalf("expected conversation_id")
	}

	// Verify 3-way consistency
	groupID := resp.Group["id"].(string)
	convID := resp.ConversationID

	// groups table
	var currentMembers int
	db.QueryRowContext(context.Background(),
		"SELECT current_members FROM groups WHERE id=$1", groupID).Scan(&currentMembers)
	if currentMembers != 1 {
		t.Fatalf("expected 1 member in groups.current_members, got %d", currentMembers)
	}

	// group_members table
	var role string
	db.QueryRowContext(context.Background(),
		"SELECT role FROM group_members WHERE group_id=$1 AND user_id=$2 AND left_at IS NULL",
		groupID, userA).Scan(&role)
	if role != "owner" {
		t.Fatalf("expected owner role, got %s", role)
	}

	// conversation exists
	var convType string
	db.QueryRowContext(context.Background(),
		"SELECT type FROM conversations WHERE id=$1", convID).Scan(&convType)
	if convType != "group" {
		t.Fatalf("expected conversation type 'group', got %s", convType)
	}

	// conversation_members
	var memberExists bool
	db.QueryRowContext(context.Background(),
		"SELECT EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id=$1 AND user_id=$2)",
		convID, userA).Scan(&memberExists)
	if !memberExists {
		t.Fatalf("expected creator to be in conversation_members")
	}

	// group_events
	var eventCount int
	db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM group_events WHERE group_id=$1 AND event_type='created'",
		groupID).Scan(&eventCount)
	if eventCount != 1 {
		t.Fatalf("expected 1 'created' group_event, got %d", eventCount)
	}

	// conversation_summary initialized
	var summaryExists bool
	db.QueryRowContext(context.Background(),
		"SELECT EXISTS(SELECT 1 FROM conversation_summaries WHERE user_id=$1 AND conversation_id=$2)",
		userA, convID).Scan(&summaryExists)
	if !summaryExists {
		t.Fatalf("expected conversation_summary for creator")
	}

	// Cleanup group data
	t.Cleanup(func() {
		db.Exec("DELETE FROM group_events WHERE group_id=$1", groupID)
		db.Exec("DELETE FROM conversation_summaries WHERE conversation_id=$1", convID)
		db.Exec("DELETE FROM conversation_members WHERE conversation_id=$1", convID)
		db.Exec("DELETE FROM conversations WHERE id=$1", convID)
		db.Exec("DELETE FROM group_members WHERE group_id=$1", groupID)
		db.Exec("DELETE FROM groups WHERE id=$1", groupID)
	})
}

func TestAddMembersUpdatesGroupAndEmitsEvents(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	userA := uniqueUserID(t, 81)
	userB := uniqueUserID(t, 82)
	userC := uniqueUserID(t, 83)
	deviceA := uniqueDeviceID(t, "ios-addr-a")
	ensureAPIUsers(t, db, []apiUserSeed{
		{userID: userA, displayName: "Admin"},
		{userID: userB, displayName: "Member B"},
		{userID: userC, displayName: "Member C"},
	})
	seedAPIDevice(t, db, userA, deviceA, "ios")
	t.Cleanup(func() {
		cleanupAPIUsers(t, db, []int64{userA, userB, userC}, []string{deviceA})
	})

	tokenA, _ := authSvc.SignAccessToken(userA, deviceA, 1)

	// Create group
	createRec := doJSONRequest(t, router, http.MethodPost, "/v1/groups", map[string]any{
		"name": "Add Members Test",
	}, tokenA)
	var createResp struct {
		Group          map[string]any `json:"group"`
		ConversationID string         `json:"conversation_id"`
	}
	json.NewDecoder(createRec.Body).Decode(&createResp)
	groupID := createResp.Group["id"].(string)
	convID := createResp.ConversationID

	t.Cleanup(func() {
		db.Exec("DELETE FROM group_events WHERE group_id=$1", groupID)
		db.Exec("DELETE FROM conversation_summaries WHERE conversation_id=$1", convID)
		db.Exec("DELETE FROM conversation_members WHERE conversation_id=$1", convID)
		db.Exec("DELETE FROM conversations WHERE id=$1", convID)
		db.Exec("DELETE FROM group_members WHERE group_id=$1", groupID)
		db.Exec("DELETE FROM groups WHERE id=$1", groupID)
	})

	// Add members B + C
	addRec := doJSONRequest(t, router, http.MethodPost, "/v1/groups/"+groupID+"/members", map[string]any{
		"user_ids": []int64{userB, userC},
	}, tokenA)
	if addRec.Code != http.StatusOK {
		t.Fatalf("expected add members 200, got %d: %s", addRec.Code, addRec.Body.String())
	}

	// Verify count
	var currentMembers int
	db.QueryRowContext(context.Background(),
		"SELECT current_members FROM groups WHERE id=$1", groupID).Scan(&currentMembers)
	if currentMembers != 3 {
		t.Fatalf("expected 3 members, got %d", currentMembers)
	}

	// Verify B and C have conversation_summary
	for _, uid := range []int64{userB, userC} {
		var ok bool
		db.QueryRowContext(context.Background(),
			"SELECT EXISTS(SELECT 1 FROM conversation_summaries WHERE user_id=$1 AND conversation_id=$2)",
			uid, convID).Scan(&ok)
		if !ok {
			t.Fatalf("expected conversation_summary for user %d", uid)
		}
	}

	// Verify group_events
	var joinEvents int
	db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM group_events WHERE group_id=$1 AND event_type='member_joined'",
		groupID).Scan(&joinEvents)
	if joinEvents != 2 {
		t.Fatalf("expected 2 member_joined events, got %d", joinEvents)
	}
}

func TestLeaveGroupHidesConversationAndPreventsMessages(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	userA := uniqueUserID(t, 91)
	userB := uniqueUserID(t, 92)
	deviceA := "ios-leave-a"
	deviceB := "ios-leave-b"
	ensureAPIUsers(t, db, []apiUserSeed{
		{userID: userA, displayName: "Owner"},
		{userID: userB, displayName: "Leaver"},
	})
	seedAPIDevice(t, db, userA, deviceA, "ios")
	seedAPIDevice(t, db, userB, deviceB, "ios")
	t.Cleanup(func() {
		cleanupAPIUsers(t, db, []int64{userA, userB}, []string{deviceA, deviceB})
	})

	tokenA, _ := authSvc.SignAccessToken(userA, deviceA, 1)
	tokenB, _ := authSvc.SignAccessToken(userB, deviceB, 1)

	// Create group + add B
	createRec := doJSONRequest(t, router, http.MethodPost, "/v1/groups", map[string]any{"name": "Leave Test"}, tokenA)
	var createResp struct {
		Group          map[string]any `json:"group"`
		ConversationID string         `json:"conversation_id"`
	}
	json.NewDecoder(createRec.Body).Decode(&createResp)
	groupID := createResp.Group["id"].(string)
	convID := createResp.ConversationID

	doJSONRequest(t, router, http.MethodPost, "/v1/groups/"+groupID+"/members", map[string]any{
		"user_ids": []int64{userB},
	}, tokenA)

	// B leaves the group
	leaveRec := doJSONRequest(t, router, http.MethodPost, "/v1/groups/"+groupID+"/leave", nil, tokenB)
	if leaveRec.Code != http.StatusOK {
		t.Fatalf("expected leave 200, got %d: %s", leaveRec.Code, leaveRec.Body.String())
	}

	// Verify is_hidden for B
	var isHidden bool
	db.QueryRowContext(context.Background(),
		"SELECT is_hidden FROM conversation_summaries WHERE user_id=$1 AND conversation_id=$2",
		userB, convID).Scan(&isHidden)
	if !isHidden {
		t.Fatalf("expected is_hidden=true for leaver")
	}

	t.Cleanup(func() {
		db.Exec("DELETE FROM group_events WHERE group_id=$1", groupID)
		db.Exec("DELETE FROM conversation_summaries WHERE conversation_id=$1", convID)
		db.Exec("DELETE FROM conversation_members WHERE conversation_id=$1", convID)
		db.Exec("DELETE FROM conversations WHERE id=$1", convID)
		db.Exec("DELETE FROM group_members WHERE group_id=$1", groupID)
		db.Exec("DELETE FROM groups WHERE id=$1", groupID)
	})
}

func TestRemoveMemberRequiresAdminAndCannotRemoveOwner(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	userA := uniqueUserID(t, 101)
	userB := uniqueUserID(t, 102)
	userC := uniqueUserID(t, 103)
	deviceA := "ios-remove-a"
	deviceB := "ios-remove-b"
	deviceC := "ios-remove-c"
	ensureAPIUsers(t, db, []apiUserSeed{
		{userID: userA, displayName: "Owner"},
		{userID: userB, displayName: "Regular"},
		{userID: userC, displayName: "AlsoRegular"},
	})
	seedAPIDevice(t, db, userA, deviceA, "ios")
	seedAPIDevice(t, db, userB, deviceB, "ios")
	seedAPIDevice(t, db, userC, deviceC, "ios")
	t.Cleanup(func() {
		cleanupAPIUsers(t, db, []int64{userA, userB, userC}, []string{deviceA, deviceB, deviceC})
	})

	tokenA, _ := authSvc.SignAccessToken(userA, deviceA, 1)
	tokenB, _ := authSvc.SignAccessToken(userB, deviceB, 1)

	// Create group + add B and C
	createRec := doJSONRequest(t, router, http.MethodPost, "/v1/groups", map[string]any{"name": "Remove Test"}, tokenA)
	var createResp struct {
		Group          map[string]any `json:"group"`
		ConversationID string         `json:"conversation_id"`
	}
	json.NewDecoder(createRec.Body).Decode(&createResp)
	groupID := createResp.Group["id"].(string)
	convID := createResp.ConversationID

	doJSONRequest(t, router, http.MethodPost, "/v1/groups/"+groupID+"/members", map[string]any{
		"user_ids": []int64{userB, userC},
	}, tokenA)

	// Regular member B tries to remove C — should fail
	rec := doJSONRequest(t, router, http.MethodDelete, "/v1/groups/"+groupID+"/members/"+fmt.Sprintf("%d", userC), nil, tokenB)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected non-admin removal 403, got %d: %s", rec.Code, rec.Body.String())
	}

	// Owner A tries to remove self (owner) — should fail
	rec = doJSONRequest(t, router, http.MethodDelete, "/v1/groups/"+groupID+"/members/"+fmt.Sprintf("%d", userA), nil, tokenA)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected remove owner 409, got %d: %s", rec.Code, rec.Body.String())
	}

	// Owner A removes C — should succeed
	rec = doJSONRequest(t, router, http.MethodDelete, "/v1/groups/"+groupID+"/members/"+fmt.Sprintf("%d", userC), nil, tokenA)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected remove C 200, got %d: %s", rec.Code, rec.Body.String())
	}

	t.Cleanup(func() {
		db.Exec("DELETE FROM group_events WHERE group_id=$1", groupID)
		db.Exec("DELETE FROM conversation_summaries WHERE conversation_id=$1", convID)
		db.Exec("DELETE FROM conversation_members WHERE conversation_id=$1", convID)
		db.Exec("DELETE FROM conversations WHERE id=$1", convID)
		db.Exec("DELETE FROM group_members WHERE group_id=$1", groupID)
		db.Exec("DELETE FROM groups WHERE id=$1", groupID)
	})
}

// ── New 0011 tests ─────────────────────────────────────

func TestRequestCodeReturnsRetryAfterAndExpiresIn(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	phone := uniquePhone(t)

	rec := doJSONRequest(t, router, http.MethodPost, "/v1/auth/request_code", map[string]any{
		"phone_e164": phone,
	}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected request_code 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		RetryAfterSec int `json:"retry_after_sec"`
		ExpiresInSec  int `json:"expires_in_sec"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode request_code response: %v", err)
	}
	if resp.RetryAfterSec != 30 {
		t.Fatalf("expected retry_after_sec=30, got %d", resp.RetryAfterSec)
	}
	if resp.ExpiresInSec != 300 {
		t.Fatalf("expected expires_in_sec=300, got %d", resp.ExpiresInSec)
	}
}

func TestRequestCodeRejectsInvalidE164Format(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	invalidPhones := []string{
		"15551234567", // no leading +
		"+123",        // too short (<7 digits)
		"+",           // just +
		"not-a-phone",
	}
	for _, phone := range invalidPhones {
		rec := doJSONRequest(t, router, http.MethodPost, "/v1/auth/request_code", map[string]any{
			"phone_e164": phone,
		}, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected phone %q to get 400, got %d: %s", phone, rec.Code, rec.Body.String())
		}
	}
}

func TestVerifyCodeCreatesNewUserAndReturnsTokens(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	phone := uniquePhone(t)
	deviceID := uniqueDeviceID(t, "ios-vc")

	// Step 1: Request code
	rec := doJSONRequest(t, router, http.MethodPost, "/v1/auth/request_code", map[string]any{"phone_e164": phone}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("request_code: %d: %s", rec.Code, rec.Body.String())
	}

	// Step 2: Verify code
	rec = doJSONRequest(t, router, http.MethodPost, "/v1/auth/verify_code", map[string]any{
		"phone_e164":        phone,
		"verification_code": "123456",
		"device_id":         deviceID,
		"platform":          "ios",
	}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("verify_code: %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		UserID       int64  `json:"user_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode verify_code: %v", err)
	}
	if resp.AccessToken == "" {
		t.Fatalf("expected access_token")
	}
	if resp.RefreshToken == "" {
		t.Fatalf("expected refresh_token")
	}

	userID := userIDByPhone(t, db, phone)
	t.Cleanup(func() {
		cleanupAPIUsers(t, db, []int64{userID}, []string{deviceID})
	})

	// Token should work for protected endpoints
	protected := doJSONRequest(t, router, http.MethodGet, "/v1/devices", nil, resp.AccessToken)
	if protected.Code != http.StatusOK {
		t.Fatalf("expected devices 200 with new token, got %d: %s", protected.Code, protected.Body.String())
	}
}

func TestVerifyCodeRejectsExpiredOrMissingCode(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	phone := uniquePhone(t)
	deviceID := uniqueDeviceID(t, "ios-vc-exp")

	// Verify without requesting code first
	rec := doJSONRequest(t, router, http.MethodPost, "/v1/auth/verify_code", map[string]any{
		"phone_e164":        phone,
		"verification_code": "123456",
		"device_id":         deviceID,
		"platform":          "ios",
	}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected verify without code 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeviceRevocationRejectsFutureRequests(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	phone := uniquePhone(t)
	deviceID := uniqueDeviceID(t, "ios-revoke")

	// Register via new flow
	doJSONRequest(t, router, http.MethodPost, "/v1/auth/request_code", map[string]any{"phone_e164": phone}, "")
	verifyRec := doJSONRequest(t, router, http.MethodPost, "/v1/auth/verify_code", map[string]any{
		"phone_e164":        phone,
		"verification_code": "123456",
		"device_id":         deviceID,
		"platform":          "ios",
	}, "")
	if verifyRec.Code != http.StatusOK {
		t.Fatalf("verify_code: %d: %s", verifyRec.Code, verifyRec.Body.String())
	}

	userID := userIDByPhone(t, db, phone)
	t.Cleanup(func() {
		cleanupAPIUsers(t, db, []int64{userID}, []string{deviceID})
	})

	var tokens struct {
		AccessToken string `json:"access_token"`
	}
	json.NewDecoder(verifyRec.Body).Decode(&tokens)

	// Revoke the device
	revokeRec := doJSONRequest(t, router, http.MethodPost, "/v1/devices/"+deviceID+"/revoke", nil, tokens.AccessToken)
	if revokeRec.Code != http.StatusOK {
		t.Fatalf("revoke: %d: %s", revokeRec.Code, revokeRec.Body.String())
	}

	// Same token should now be rejected
	protected := doJSONRequest(t, router, http.MethodGet, "/v1/devices", nil, tokens.AccessToken)
	if protected.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after revoke, got %d: %s", protected.Code, protected.Body.String())
	}
}

func TestPushTokenBindingAndUpdate(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	phone := uniquePhone(t)
	deviceID := uniqueDeviceID(t, "ios-push")

	doJSONRequest(t, router, http.MethodPost, "/v1/auth/request_code", map[string]any{"phone_e164": phone}, "")
	verifyRec := doJSONRequest(t, router, http.MethodPost, "/v1/auth/verify_code", map[string]any{
		"phone_e164":        phone,
		"verification_code": "123456",
		"device_id":         deviceID,
		"platform":          "ios",
	}, "")
	if verifyRec.Code != http.StatusOK {
		t.Fatalf("verify_code: %d: %s", verifyRec.Code, verifyRec.Body.String())
	}

	userID := userIDByPhone(t, db, phone)
	t.Cleanup(func() {
		cleanupAPIUsers(t, db, []int64{userID}, []string{deviceID})
	})

	var tokens struct {
		AccessToken string `json:"access_token"`
	}
	json.NewDecoder(verifyRec.Body).Decode(&tokens)

	// Bind push token
	pushRec := doJSONRequest(t, router, http.MethodPost, "/v1/devices/push-token", map[string]any{
		"push_token": "apns-token-abc123",
		"platform":   "ios",
	}, tokens.AccessToken)
	if pushRec.Code != http.StatusOK {
		t.Fatalf("push-token: %d: %s", pushRec.Code, pushRec.Body.String())
	}

	// Verify in DB
	var pushToken string
	db.QueryRowContext(context.Background(),
		"SELECT push_token FROM devices WHERE id=$1 AND user_id=$2", deviceID, userID,
	).Scan(&pushToken)
	if pushToken != "apns-token-abc123" {
		t.Fatalf("expected push_token 'apns-token-abc123', got %q", pushToken)
	}
}

func TestListDevicesIncludesSessionVersion(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	phone := uniquePhone(t)
	deviceID := uniqueDeviceID(t, "ios-sv")

	doJSONRequest(t, router, http.MethodPost, "/v1/auth/request_code", map[string]any{"phone_e164": phone}, "")
	verifyRec := doJSONRequest(t, router, http.MethodPost, "/v1/auth/verify_code", map[string]any{
		"phone_e164":        phone,
		"verification_code": "123456",
		"device_id":         deviceID,
		"platform":          "ios",
	}, "")
	if verifyRec.Code != http.StatusOK {
		t.Fatalf("verify_code: %d: %s", verifyRec.Code, verifyRec.Body.String())
	}

	userID := userIDByPhone(t, db, phone)
	t.Cleanup(func() {
		cleanupAPIUsers(t, db, []int64{userID}, []string{deviceID})
	})

	var tokens struct {
		AccessToken string `json:"access_token"`
	}
	json.NewDecoder(verifyRec.Body).Decode(&tokens)

	rec := doJSONRequest(t, router, http.MethodGet, "/v1/devices", nil, tokens.AccessToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("devices: %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Devices []struct {
			DeviceID       string `json:"device_id"`
			SessionVersion int    `json:"session_version"`
			IsCurrent      bool   `json:"is_current"`
		} `json:"devices"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode devices: %v", err)
	}
	for _, d := range resp.Devices {
		if d.DeviceID == deviceID {
			if d.SessionVersion != 1 {
				t.Fatalf("expected session_version=1, got %d", d.SessionVersion)
			}
			if !d.IsCurrent {
				t.Fatalf("expected is_current=true")
			}
			return
		}
	}
	t.Fatalf("device %s not found in list", deviceID)
}

func TestOldRegisterStillWorks(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	phone := uniquePhone(t)
	deviceID := uniqueDeviceID(t, "ios-old-reg")

	rec := doJSONRequest(t, router, http.MethodPost, "/v1/auth/register", map[string]any{
		"phone_e164":        phone,
		"verification_code": "123456",
		"device_id":         deviceID,
		"platform":          "ios",
	}, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected old register 201, got %d: %s", rec.Code, rec.Body.String())
	}

	userID := userIDByPhone(t, db, phone)
	t.Cleanup(func() {
		cleanupAPIUsers(t, db, []int64{userID}, []string{deviceID})
	})

	var resp struct {
		AccessToken string `json:"access_token"`
		UserID      int64  `json:"user_id"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.AccessToken == "" || resp.UserID != userID {
		t.Fatalf("old register response: %+v", resp)
	}

	// Token should still work
	protected := doJSONRequest(t, router, http.MethodGet, "/v1/devices", nil, resp.AccessToken)
	if protected.Code != http.StatusOK {
		t.Fatalf("old register token rejected: %d: %s", protected.Code, protected.Body.String())
	}
}

// ── Phase 2/3 integration tests ──────────────────────────

func TestGroupMessageFanoutRejectsRemovedMember(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	userA := uniqueUserID(t, 201)
	userB := uniqueUserID(t, 202)
	userC := uniqueUserID(t, 203)
	deviceA := "ios-fanout-a"
	deviceB := "ios-fanout-b"
	deviceC := "ios-fanout-c"
	ensureAPIUsers(t, db, []apiUserSeed{
		{userID: userA, displayName: "Fanout A"},
		{userID: userB, displayName: "Fanout B"},
		{userID: userC, displayName: "Fanout C"},
	})
	seedAPIDevice(t, db, userA, deviceA, "ios")
	seedAPIDevice(t, db, userB, deviceB, "ios")
	seedAPIDevice(t, db, userC, deviceC, "ios")
	t.Cleanup(func() {
		cleanupAPIUsers(t, db, []int64{userA, userB, userC}, []string{deviceA, deviceB, deviceC})
	})

	tokenA, _ := authSvc.SignAccessToken(userA, deviceA, 1)
	tokenC, _ := authSvc.SignAccessToken(userC, deviceC, 1)

	// Create group + add B and C
	createRec := doJSONRequest(t, router, http.MethodPost, "/v1/groups", map[string]any{"name": "Fanout Test"}, tokenA)
	var createResp struct {
		Group          map[string]any `json:"group"`
		ConversationID string         `json:"conversation_id"`
	}
	json.NewDecoder(createRec.Body).Decode(&createResp)
	groupID := createResp.Group["id"].(string)
	convID := createResp.ConversationID

	doJSONRequest(t, router, http.MethodPost, "/v1/groups/"+groupID+"/members", map[string]any{
		"user_ids": []int64{userB, userC},
	}, tokenA)

	// A sends a message
	resp := sendMessageRequestAndDecode(t, router, tokenA, map[string]any{
		"client_message_id": "group-fanout-1",
		"conversation_id":   convID,
		"content":           `{"text":"hello group"}`,
	})
	if resp.ConversationSeq != 1 {
		t.Fatalf("expected seq 1, got %d", resp.ConversationSeq)
	}

	// Owner A removes C from the group
	rec := doJSONRequest(t, router, http.MethodDelete,
		"/v1/groups/"+groupID+"/members/"+fmt.Sprintf("%d", userC), nil, tokenA)
	if rec.Code != http.StatusOK {
		t.Fatalf("remove C: %d: %s", rec.Code, rec.Body.String())
	}

	// C should NOT be able to send to the group
	rec = doJSONRequest(t, router, http.MethodPost, "/v1/messages/send", map[string]any{
		"client_message_id": "group-fanout-forbidden",
		"conversation_id":   convID,
		"content":           `{"text":"should fail"}`,
	}, tokenC)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected removed member to get 403, got %d: %s", rec.Code, rec.Body.String())
	}

	t.Cleanup(func() {
		db.Exec("DELETE FROM group_events WHERE group_id=$1", groupID)
		db.Exec("DELETE FROM conversation_summaries WHERE conversation_id=$1", convID)
		db.Exec("DELETE FROM conversation_members WHERE conversation_id=$1", convID)
		db.Exec("DELETE FROM conversations WHERE id=$1", convID)
		db.Exec("DELETE FROM group_members WHERE group_id=$1", groupID)
		db.Exec("DELETE FROM groups WHERE id=$1", groupID)
	})
}

func TestRefreshTokenRotationWorksAfterRevocation(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	phone := uniquePhone(t)
	deviceID := uniqueDeviceID(t, "ios-refresh-revoke")

	doJSONRequest(t, router, http.MethodPost, "/v1/auth/request_code", map[string]any{"phone_e164": phone}, "")
	verifyRec := doJSONRequest(t, router, http.MethodPost, "/v1/auth/verify_code", map[string]any{
		"phone_e164":        phone,
		"verification_code": "123456",
		"device_id":         deviceID,
		"platform":          "ios",
	}, "")
	if verifyRec.Code != http.StatusOK {
		t.Fatalf("verify_code: %d: %s", verifyRec.Code, verifyRec.Body.String())
	}

	userID := userIDByPhone(t, db, phone)
	t.Cleanup(func() {
		cleanupAPIUsers(t, db, []int64{userID}, []string{deviceID})
	})

	var tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	json.NewDecoder(verifyRec.Body).Decode(&tokens)

	// Revoke the device — bumps session_version
	_ = doJSONRequest(t, router, http.MethodPost, "/v1/devices/"+deviceID+"/revoke", nil, tokens.AccessToken)

	// Refresh uses refresh_token (not JWT) — should succeed and get new session_version
	refreshRec := doJSONRequest(t, router, http.MethodPost, "/v1/auth/refresh", map[string]any{
		"refresh_token": tokens.RefreshToken,
	}, "")
	if refreshRec.Code != http.StatusOK {
		t.Fatalf("refresh after revoke should work: %d: %s", refreshRec.Code, refreshRec.Body.String())
	}

	var newTokens struct {
		AccessToken string `json:"access_token"`
	}
	json.NewDecoder(refreshRec.Body).Decode(&newTokens)
	if newTokens.AccessToken == "" {
		t.Fatalf("expected new access_token after refresh")
	}

	// New token (with bumped session_version) should work
	protected := doJSONRequest(t, router, http.MethodGet, "/v1/devices", nil, newTokens.AccessToken)
	if protected.Code != http.StatusOK {
		t.Fatalf("new token after refresh+revoke should work: %d: %s", protected.Code, protected.Body.String())
	}
}

// ── Test helpers ────────────────────────────────────────

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
	// Unique RemoteAddr avoids shared Redis per-IP OTP rate limits (20/hour) across the suite.
	req.RemoteAddr = fmt.Sprintf("127.0.0.1:%d", time.Now().UnixNano()%60000+10000)
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

func seedAPIDevice(t *testing.T, db *sql.DB, userID int64, deviceID, platform string) {
	t.Helper()
	mustExecAPI(t, db,
		`INSERT INTO devices (id, user_id, platform, session_version, last_seen_at)
		 VALUES ($1, $2, $3, 1, NOW())
		 ON CONFLICT (id) DO NOTHING`,
		deviceID, userID, platform,
	)
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
	mustExecAPI(t, db, `DELETE FROM login_audit_events WHERE user_id = ANY($1)`, pqInt64Array(userIDs))
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

func seedConversationMessages(t *testing.T, db *sql.DB, conversationID string, senderUserID int64, senderDeviceID string, seqs []int64) {
	t.Helper()
	for _, seq := range seqs {
		serverMessageID := fmt.Sprintf("msg-%s-%d", conversationID, seq)
		clientMessageID := fmt.Sprintf("client-%s-%d", conversationID, seq)
		content := fmt.Sprintf(`{"text":"message-%d"}`, seq)
		mustExecAPI(t, db,
			`INSERT INTO messages (
				server_message_id, conversation_id, conversation_seq, sender_user_id,
				sender_device_id, client_message_id, message_type, content, server_received_at
			) VALUES ($1, $2, $3, $4, $5, $6, 'text', $7::jsonb, NOW())`,
			serverMessageID, conversationID, seq, senderUserID, senderDeviceID, clientMessageID, content,
		)
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
