package fanout

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/domain"
)

// ErrGroupBusy is returned when a hot group is being rate-limited.
var ErrGroupBusy = errors.New("group is busy, retry later")

// SmallGroupThreshold is the max member count for full write-amplification.
const SmallGroupThreshold = 50

// MediumGroupThreshold is the max member count for hybrid fanout.
const MediumGroupThreshold = 200

// HotGroupMsgThreshold is the message count in a 60s window that triggers hot-group protection.
const HotGroupMsgThreshold = 50

// Deliverer is the interface for delivering a message frame to a device.
// Gateway implements this via gRPC.
type Deliverer interface {
	DeliverMessage(ctx context.Context, userID int64, deviceID string, payload *DeliveryPayload) error
}

// DeliveryPayload carries the data needed to construct a MESSAGE_DELIVERY frame.
type DeliveryPayload struct {
	ServerMessageID    string `json:"server_message_id"`
	ConversationID     string `json:"conversation_id"`
	ConversationSeq    int64  `json:"conversation_seq"`
	SenderUserID       int64  `json:"sender_user_id"`
	SenderDeviceID     string `json:"sender_device_id"`
	MessageType        string `json:"message_type"`
	Content            string `json:"content"`
	ServerReceivedAtMs int64  `json:"server_received_at_ms"`
	TraceID            string `json:"trace_id"`
}

// PushNotifier is the interface for triggering push notifications after fanout.
type PushNotifier interface {
	NotifyOffline(ctx context.Context, userID int64, conversationID string, conversationSeq int64, serverMessageID string, latestEventSeq int64, senderName string) error
}
type SyncWriter interface {
	AppendEvent(ctx context.Context, userID int64, eventType string, payload []byte) error
	AppendEventWithConv(ctx context.Context, userID int64, conversationID, eventType string, payload []byte) error
}

// Service handles message fanout to online devices and sync backfill for offline ones.
type Service struct {
	db        *sql.DB
	rdb       *redis.Client
	deliverer Deliverer
	sync      SyncWriter
	push      PushNotifier
}

func NewService(db *sql.DB, rdb *redis.Client, deliverer Deliverer, sync SyncWriter) *Service {
	return &Service{db: db, rdb: rdb, deliverer: deliverer, sync: sync}
}

// SetPushNotifier optionally enables push notification after fanout.
func (s *Service) SetPushNotifier(p PushNotifier) {
	s.push = p
}

// Fanout delivers a message to all conversation members' online devices
// and writes sync events for offline devices.
// For 1:1 conversations, behavior is unchanged from Phase 1.
// For group conversations, uses tiered strategy based on member count.
func (s *Service) Fanout(ctx context.Context, event domain.OutboxEvent) error {
	var p struct {
		ServerMessageID    string `json:"server_message_id"`
		ConversationID     string `json:"conversation_id"`
		ConversationSeq    int64  `json:"conversation_seq"`
		SenderUserID       int64  `json:"sender_user_id"`
		SenderDeviceID     string `json:"sender_device_id"`
		MessageType        string `json:"message_type"`
		Content            string `json:"content"`
		ServerReceivedAtMs int64  `json:"server_received_at_ms"`
		CreatedAt          string `json:"created_at"`
		TraceID            string `json:"trace_id"`
	}
	if err := json.Unmarshal([]byte(event.Payload), &p); err != nil {
		return err
	}

	// Hot group check (only for group conversations)
	convType, groupID := s.conversationType(ctx, p.ConversationID)
	if convType == "group" && s.isHotGroup(ctx, groupID) {
		return ErrGroupBusy
	}

	// Get conversation members
	members, err := s.resolveTargets(ctx, p.ConversationID, p.SenderUserID)
	if err != nil {
		return err
	}

	memberCount := len(members)

	delivery := &DeliveryPayload{
		ServerMessageID:    p.ServerMessageID,
		ConversationID:     p.ConversationID,
		ConversationSeq:    p.ConversationSeq,
		SenderUserID:       p.SenderUserID,
		SenderDeviceID:     p.SenderDeviceID,
		MessageType:        p.MessageType,
		Content:            p.Content,
		ServerReceivedAtMs: p.ServerReceivedAtMs,
		TraceID:            p.TraceID,
	}

	// Track hot group for sliding window (group only)
	if convType == "group" {
		s.trackGroupMessage(ctx, groupID, p.ServerMessageID)
	}

	// Tiered fanout strategy
	for _, memberID := range members {
		// All members always get a sync event
		s.appendSyncEvent(ctx, memberID, p)

		// Realtime delivery strategy varies by group size
		if convType == "direct" || memberCount <= SmallGroupThreshold {
			// Small group / direct: try realtime delivery for online devices
			s.deliverToOnlineDevices(ctx, memberID, delivery)
		} else if memberCount <= MediumGroupThreshold {
			// Medium group: online only
			s.deliverToOnlineDevices(ctx, memberID, delivery)
		}
		// Large group (>200): sync-only, no realtime delivery

		// After fanout: if member is offline, trigger push notification
		if s.push != nil {
			_ = s.push.NotifyOffline(ctx, memberID, p.ConversationID, p.ConversationSeq,
				p.ServerMessageID, 0 /* latestEventSeq filled by push orchestrator */, "")
		}
	}

	return nil
}

// resolveTargets returns member user IDs for the conversation, excluding the sender.
// For 1:1 conversations it uses conversation_members.
// For group conversations it uses group_members (active only).
func (s *Service) resolveTargets(ctx context.Context, conversationID string, senderUserID int64) ([]int64, error) {
	// Try group members first
	groupID := strings.TrimPrefix(conversationID, "conv_grp_")
	rows, err := s.db.QueryContext(ctx,
		`SELECT user_id FROM group_members
		 WHERE group_id=$1 AND left_at IS NULL AND user_id != $2`,
		groupID, senderUserID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(ids) > 0 {
		return ids, nil
	}

	// Fallback to conversation_members for 1:1
	rows2, err := s.db.QueryContext(ctx,
		"SELECT user_id FROM conversation_members WHERE conversation_id=$1 AND user_id != $2",
		conversationID, senderUserID,
	)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()

	ids = ids[:0]
	for rows2.Next() {
		var id int64
		if err := rows2.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows2.Err()
}

func (s *Service) deliverToOnlineDevices(ctx context.Context, userID int64, delivery *DeliveryPayload) {
	devices, err := s.getOnlineDevices(ctx, userID)
	if err != nil {
		slog.Error("get online devices", "user_id", userID, "error", err)
		return
	}
	for _, did := range devices {
		err := s.deliverer.DeliverMessage(ctx, userID, did, delivery)
		if err != nil {
			slog.Debug("deliver failed, fallback to sync",
				"user_id", userID, "device_id", did, "error", err)
			continue
		}
	}
}

// ── Hot group detection ───────────────────────────

func (s *Service) isHotGroup(ctx context.Context, groupID string) bool {
	if s.rdb == nil {
		return false
	}
	key := "hot_group:" + groupID
	card, err := s.rdb.ZCard(ctx, key).Result()
	if err != nil {
		return false
	}
	return card > HotGroupMsgThreshold
}

func (s *Service) trackGroupMessage(ctx context.Context, groupID, messageID string) {
	if s.rdb == nil {
		return
	}
	key := "hot_group:" + groupID
	now := time.Now().UnixMilli()
	cutoff := now - 60_000 // 60 second sliding window

	pipe := s.rdb.Pipeline()
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: messageID})
	pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(cutoff, 10))
	pipe.Expire(ctx, key, 120*time.Second)
	_, _ = pipe.Exec(ctx)
}

// ── Conversation type lookup ───────────────────────

func (s *Service) conversationType(ctx context.Context, conversationID string) (string, string) {
	var convType string
	var groupID string
	err := s.db.QueryRowContext(ctx,
		"SELECT type FROM conversations WHERE id=$1", conversationID,
	).Scan(&convType)
	if err != nil {
		return "direct", ""
	}
	if convType == "group" {
		groupID = strings.TrimPrefix(conversationID, "conv_grp_")
	}
	return convType, groupID
}

// ── Online device lookup ───────────────────────────

func (s *Service) getOnlineDevices(ctx context.Context, userID int64) ([]string, error) {
	pattern := redisUserPattern(userID)
	keys, err := s.rdb.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, err
	}

	devices := make([]string, 0, len(keys))
	for _, key := range keys {
		parts := strings.Split(key, ":")
		if len(parts) >= 4 {
			devices = append(devices, parts[3])
		}
	}
	return devices, nil
}

// ── Sync event helpers ────────────────────────────

func (s *Service) appendSyncEvent(ctx context.Context, userID int64, p struct {
	ServerMessageID    string `json:"server_message_id"`
	ConversationID     string `json:"conversation_id"`
	ConversationSeq    int64  `json:"conversation_seq"`
	SenderUserID       int64  `json:"sender_user_id"`
	SenderDeviceID     string `json:"sender_device_id"`
	MessageType        string `json:"message_type"`
	Content            string `json:"content"`
	ServerReceivedAtMs int64  `json:"server_received_at_ms"`
	CreatedAt          string `json:"created_at"`
	TraceID            string `json:"trace_id"`
}) {
	payload := map[string]interface{}{
		"server_message_id":     p.ServerMessageID,
		"conversation_id":       p.ConversationID,
		"conversation_seq":      p.ConversationSeq,
		"sender_user_id":        p.SenderUserID,
		"sender_device_id":      p.SenderDeviceID,
		"message_type":          p.MessageType,
		"content":               p.Content,
		"server_received_at_ms": p.ServerReceivedAtMs,
		"trace_id":              p.TraceID,
	}
	payloadJSON, _ := json.Marshal(payload)
	if err := s.sync.AppendEvent(ctx, userID, domain.EventTypeMessageCreated, payloadJSON); err != nil {
		slog.Error("append sync event", "user_id", userID, "error", err)
	}
}

func redisUserPattern(userID int64) string {
	return "gateway:user:" + strconv.FormatInt(userID, 10) + ":*"
}

// ── Backward compat (used by messages.Service) ─────

// ErrNotGroupMember is returned when a user tries to send to a group they're not in.
var ErrNotGroupMember = fmt.Errorf("not a group member")
