package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/auth"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/backpressure"
)

// TestSendMessageBackpressureRejectsThenRecovers is the API-level contract for
// ticket 0032: while the outbox backlog is above the threshold, send is shed
// with 429 + Retry-After; once the backlog drains, sends succeed again.
func TestSendMessageBackpressureRejectsThenRecovers(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)

	limiter := backpressure.NewLimiter(db, backpressure.Config{
		PendingThreshold: 10,
		RetryAfter:       4 * time.Second,
		SampleInterval:   time.Hour, // no background sampling; test drives the sample
	})
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil,
		WithSendLimiter(limiter))

	convID := uniqueConversationID(t, "send-backpressure")
	userA := uniqueUserID(t, 301)
	userB := uniqueUserID(t, 302)
	seedAPIDirectConversation(t, db, convID, []apiUserSeed{
		{userID: userA, displayName: "A"},
		{userID: userB, displayName: "B"},
	})
	t.Cleanup(func() { cleanupAPIConversation(t, db, convID, []int64{userA, userB}) })

	deviceA := uniqueDeviceID(t, "ios-bp-a")
	seedAPIDevice(t, db, userA, deviceA, "ios")
	tokenA, err := authSvc.SignAccessToken(userA, deviceA, 1)
	if err != nil {
		t.Fatalf("SignAccessToken: %v", err)
	}

	body := func(id string) map[string]any {
		return map[string]any{
			"client_message_id": id,
			"conversation_id":   convID,
			"message_type":      "text",
			"content":           `{"text":"backpressure"}`,
		}
	}

	// Backlog below threshold: normal behaviour.
	limiter.SetPending(0)
	rec := doJSONRequest(t, router, http.MethodPost, "/v1/messages/send", body("bp-ok-1"), tokenA)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected send 200 under threshold, got %d: %s", rec.Code, rec.Body.String())
	}

	// Backlog above threshold: shed the write.
	limiter.SetPending(50)
	rec = doJSONRequest(t, router, http.MethodPost, "/v1/messages/send", body("bp-shed-1"), tokenA)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected send 429 over threshold, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Retry-After"); got != "4" {
		t.Fatalf("expected Retry-After=4, got %q", got)
	}

	// The shed request must not have written anything.
	assertMessageCount(t, db, convID, 1)
	assertOutboxCount(t, db, convID, 1)

	// Consumer catches up: sends resume without a restart.
	limiter.SetPending(1)
	rec = doJSONRequest(t, router, http.MethodPost, "/v1/messages/send", body("bp-ok-2"), tokenA)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected send 200 after backlog drained, got %d: %s", rec.Code, rec.Body.String())
	}
	assertMessageCount(t, db, convID, 2)
	assertOutboxCount(t, db, convID, 2)
}

// TestSendMessageWithoutLimiterKeepsLegacyBehaviour guards the opt-in nature of
// the limiter: routers built without the option behave exactly as before.
func TestSendMessageWithoutLimiterKeepsLegacyBehaviour(t *testing.T) {
	db := openAPITestDB(t)
	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	router := NewRouter(db, redis.NewClient(&redis.Options{Addr: "localhost:6379"}), authSvc, nil)

	convID := uniqueConversationID(t, "send-no-bp")
	userA := uniqueUserID(t, 311)
	userB := uniqueUserID(t, 312)
	seedAPIDirectConversation(t, db, convID, []apiUserSeed{
		{userID: userA, displayName: "A"},
		{userID: userB, displayName: "B"},
	})
	t.Cleanup(func() { cleanupAPIConversation(t, db, convID, []int64{userA, userB}) })

	deviceA := uniqueDeviceID(t, "ios-nobp-a")
	seedAPIDevice(t, db, userA, deviceA, "ios")
	tokenA, err := authSvc.SignAccessToken(userA, deviceA, 1)
	if err != nil {
		t.Fatalf("SignAccessToken: %v", err)
	}

	rec := doJSONRequest(t, router, http.MethodPost, "/v1/messages/send", map[string]any{
		"client_message_id": "nobp-1",
		"conversation_id":   convID,
		"message_type":      "text",
		"content":           `{"text":"no limiter"}`,
	}, tokenA)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected send 200 without limiter, got %d: %s", rec.Code, rec.Body.String())
	}
}
