package push

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// APNsClient is the interface for sending push notifications.
// Mock implementation does not actually call Apple servers.
type APNsClient interface {
	Send(ctx context.Context, deviceToken string, payload *APNsPayload) (apnsID string, status string, err error)
}

// APNsPayload represents a push notification payload.
type APNsPayload struct {
	APS      APNsAPS  `json:"aps"`
	SyncTrigger SyncTrigger `json:"sync_trigger,omitempty"`
}

type APNsAPS struct {
	ContentAvailable int    `json:"content-available,omitempty"` // 1 for silent push
	Alert            *Alert `json:"alert,omitempty"`
	Sound            string `json:"sound,omitempty"`
	Badge            int    `json:"badge,omitempty"`
	MutableContent   int    `json:"mutable-content,omitempty"`
	Priority         int    `json:"apns-priority,omitempty"`
	PushType         string `json:"apns-push-type,omitempty"`
}

type Alert struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type SyncTrigger struct {
	LatestEventSeq int64  `json:"latest_event_seq"`
	Reason         string `json:"reason"`
}

// ── Mock APNs Client ──────────────────────────────

type MockAPNsClient struct{}

func NewMockAPNsClient() *MockAPNsClient {
	return &MockAPNsClient{}
}

func (c *MockAPNsClient) Send(_ context.Context, deviceToken string, _ *APNsPayload) (string, string, error) {
	apnsID := fmt.Sprintf("apns-mock-%d", time.Now().UnixNano())
	slog.Info("mock APNs push sent", "device_token_prefix", deviceToken[:min(8, len(deviceToken))], "apns_id", apnsID)
	return apnsID, "sent", nil
}

// ── Orchestrator ──────────────────────────────────

type Orchestrator struct {
	db      *sql.DB
	rdb     *redis.Client
	apns    APNsClient
}

func NewOrchestrator(db *sql.DB, rdb *redis.Client, apns APNsClient) *Orchestrator {
	return &Orchestrator{db: db, rdb: rdb, apns: apns}
}

const (
	// CoalesceWindow is the time window for merging visible push notifications.
	CoalesceWindow = 30 * time.Second
	// SilentToVisibleUpgrade is the time after a silent push to try a visible push.
	SilentToVisibleUpgrade = 60 * time.Second
)

// Decision indicates what push action to take.
type Decision string

const (
	DecisionNone    Decision = "none"
	DecisionSilent  Decision = "silent"
	DecisionVisible Decision = "visible"
)

// PushRequest contains all the information needed to decide and send a push.
type PushRequest struct {
	UserID          int64
	ConversationID  string
	ConversationSeq int64
	ServerMessageID string
	LatestEventSeq  int64
	SenderName      string
	IsMuted         bool // if true, skip visible push
}

// DecideAndPush evaluates whether to push, what type, and sends if needed.
// Returns the decision made and any error.
func (o *Orchestrator) DecideAndPush(ctx context.Context, req PushRequest) (Decision, error) {
	if req.IsMuted {
		return DecisionNone, nil
	}

	// Find device push tokens for the user
	devices, err := o.getDevices(ctx, req.UserID)
	if err != nil {
		return DecisionNone, err
	}
	if len(devices) == 0 {
		return DecisionNone, nil
	}

	// Check coalesce window for visible push
	windowKey := fmt.Sprintf("push_window:%d:%s", req.UserID, req.ConversationID)
	windowData, err := o.rdb.Get(ctx, windowKey).Result()
	if err == nil {
		// Existing window: increment count, decide
		var win struct {
			PushType     string `json:"push_type"`
			LastPushAtMs int64  `json:"last_push_at_ms"`
			MsgCount     int    `json:"msg_count"`
		}
		json.Unmarshal([]byte(windowData), &win)
		win.MsgCount++
		win.LastPushAtMs = time.Now().UnixMilli()

		// If visible was already sent, don't send another within window
		if win.PushType == "visible" {
			data, _ := json.Marshal(win)
			o.rdb.Set(ctx, windowKey, string(data), CoalesceWindow)
			return DecisionNone, nil
		}

		// Silent push already sent within CoalesceWindow — upgrade to visible if past threshold
		if win.PushType == "silent" && time.Now().UnixMilli()-win.LastPushAtMs > SilentToVisibleUpgrade.Milliseconds() {
			win.PushType = "visible"
			data, _ := json.Marshal(win)
			o.rdb.Set(ctx, windowKey, string(data), CoalesceWindow)
			for _, d := range devices {
				o.sendVisible(ctx, d.PushToken, req, win.MsgCount)
			}
			return DecisionVisible, nil
		}

		// Still within silent window — no further action
		data, _ := json.Marshal(win)
		o.rdb.Set(ctx, windowKey, string(data), CoalesceWindow)
		return DecisionNone, nil
	}

	// No existing window: send silent push first (best-effort, lower battery impact)
	win := struct {
		PushType     string `json:"push_type"`
		LastPushAtMs int64  `json:"last_push_at_ms"`
		MsgCount     int    `json:"msg_count"`
	}{PushType: "silent", LastPushAtMs: time.Now().UnixMilli(), MsgCount: 1}
	data, _ := json.Marshal(win)
	o.rdb.Set(ctx, windowKey, string(data), CoalesceWindow)

	for _, d := range devices {
		o.sendSilent(ctx, d.PushToken, req)
	}

	return DecisionSilent, nil
}

type deviceInfo struct {
	PushToken string
	DeviceID  string
}

func (o *Orchestrator) getDevices(ctx context.Context, userID int64) ([]deviceInfo, error) {
	rows, err := o.db.QueryContext(ctx,
		"SELECT id, push_token FROM devices WHERE user_id=$1 AND push_token IS NOT NULL AND push_token != ''",
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []deviceInfo
	for rows.Next() {
		var d deviceInfo
		if err := rows.Scan(&d.DeviceID, &d.PushToken); err != nil {
			continue
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

func (o *Orchestrator) sendSilent(ctx context.Context, pushToken string, req PushRequest) {
	badge := o.calculateBadge(ctx, req.UserID)
	payload := &APNsPayload{
		APS: APNsAPS{
			ContentAvailable: 1,
			Badge:            badge,
			Priority:         5,
		},
		SyncTrigger: SyncTrigger{
			LatestEventSeq: req.LatestEventSeq,
			Reason:         "new_message",
		},
	}
	apnsID, status, err := o.apns.Send(ctx, pushToken, payload)
	if err != nil {
		slog.Error("silent push failed", "user_id", req.UserID, "error", err)
	}
	o.recordPushEvent(ctx, req.UserID, pushToken, "silent", req.ConversationID, req.ServerMessageID, apnsID, status)
}

func (o *Orchestrator) sendVisible(ctx context.Context, pushToken string, req PushRequest, msgCount int) {
	body := "🔒 新消息"
	title := req.SenderName
	if msgCount > 1 {
		body = fmt.Sprintf("🔒 %d 条新消息", msgCount)
	}
	badge := o.calculateBadge(ctx, req.UserID)
	payload := &APNsPayload{
		APS: APNsAPS{
			Alert:  &Alert{Title: title, Body: body},
			Sound:  "default",
			Badge:  badge,
			PushType: "alert",
		},
		SyncTrigger: SyncTrigger{
			LatestEventSeq: req.LatestEventSeq,
			Reason:         "new_message",
		},
	}
	apnsID, status, err := o.apns.Send(ctx, pushToken, payload)
	if err != nil {
		slog.Error("visible push failed", "user_id", req.UserID, "error", err)
	}
	o.recordPushEvent(ctx, req.UserID, pushToken, "visible", req.ConversationID, req.ServerMessageID, apnsID, status)
}

func (o *Orchestrator) calculateBadge(ctx context.Context, userID int64) int {
	var count int
	o.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(unread_count), 0)
		 FROM conversation_summaries
		 WHERE user_id=$1 AND is_hidden IS FALSE`,
		userID,
	).Scan(&count)
	return count
}

func (o *Orchestrator) recordPushEvent(ctx context.Context, userID int64, deviceID, pushType, conversationID, messageID, apnsID, status string) {
	_, err := o.db.ExecContext(ctx,
		`INSERT INTO push_events (user_id, device_id, push_type, conversation_id, message_id, apns_response, apns_status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())`,
		userID, deviceID, pushType, conversationID, messageID, apnsID, status,
	)
	if err != nil {
		slog.Error("record push event", "error", err)
	}
}

// HandleBadDeviceToken marks a push token as invalid (APNs 410).
func (o *Orchestrator) HandleBadDeviceToken(ctx context.Context, pushToken string) error {
	result, err := o.db.ExecContext(ctx,
		"UPDATE devices SET push_token = '' WHERE push_token = $1", pushToken)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	slog.Warn("marked push token invalid",
		"push_token_prefix", pushToken[:min(8, len(pushToken))],
		"devices_updated", rows)
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ── Unused import suppression ─────────────────────
var _ = strconv.Itoa
