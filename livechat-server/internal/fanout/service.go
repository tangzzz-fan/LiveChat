package fanout

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/domain"
)

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
}

// SyncWriter is the interface for appending sync events for offline devices.
type SyncWriter interface {
	AppendEvent(ctx context.Context, userID int64, eventType string, payload []byte) error
}

// Service handles message fanout to online devices and sync backfill for offline ones.
type Service struct {
	db        *sql.DB
	rdb       *redis.Client
	deliverer Deliverer
	sync      SyncWriter
}

func NewService(db *sql.DB, rdb *redis.Client, deliverer Deliverer, sync SyncWriter) *Service {
	return &Service{db: db, rdb: rdb, deliverer: deliverer, sync: sync}
}

// Fanout delivers a message to all conversation members' online devices
// and writes sync events for offline devices.
func (s *Service) Fanout(ctx context.Context, event domain.OutboxEvent) error {
	var p struct {
		ServerMessageID  string `json:"server_message_id"`
		ConversationID   string `json:"conversation_id"`
		ConversationSeq  int64  `json:"conversation_seq"`
		SenderUserID     int64  `json:"sender_user_id"`
		SenderDeviceID   string `json:"sender_device_id"`
		MessageType      string `json:"message_type"`
		CreatedAt        string `json:"created_at"`
	}
	if err := json.Unmarshal([]byte(event.Payload), &p); err != nil {
		return err
	}

	// Get conversation members
	members, err := s.getMembers(ctx, p.ConversationID)
	if err != nil {
		return err
	}

	delivery := &DeliveryPayload{
		ServerMessageID:    p.ServerMessageID,
		ConversationID:     p.ConversationID,
		ConversationSeq:    p.ConversationSeq,
		SenderUserID:       p.SenderUserID,
		SenderDeviceID:     p.SenderDeviceID,
		MessageType:        p.MessageType,
		Content:            "", // content fetched separately
		ServerReceivedAtMs: 0,
	}

	for _, memberID := range members {
		if memberID == p.SenderUserID {
			continue // don't deliver to sender
		}

		// Find online devices
		devices, err := s.getOnlineDevices(ctx, memberID)
		if err != nil {
			slog.Error("get online devices", "user_id", memberID, "error", err)
			// Fall through to sync backfill
			s.appendSyncEvent(ctx, memberID, p)
			continue
		}

		hasOnline := false
		for _, did := range devices {
			err := s.deliverer.DeliverMessage(ctx, memberID, did, delivery)
			if err != nil {
				slog.Debug("deliver failed, fallback to sync",
					"user_id", memberID, "device_id", did, "error", err)
				continue
			}
			hasOnline = true
		}

		// If no device is online, write sync event
		if !hasOnline || len(devices) == 0 {
			s.appendSyncEvent(ctx, memberID, p)
		}
	}

	return nil
}

func (s *Service) getMembers(ctx context.Context, conversationID string) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT user_id FROM conversation_members WHERE conversation_id=$1", conversationID)
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
	return ids, rows.Err()
}

func (s *Service) getOnlineDevices(ctx context.Context, userID int64) ([]string, error) {
	pattern := redisUserPattern(userID)
	keys, err := s.rdb.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, err
	}

	devices := make([]string, 0, len(keys))
	for _, key := range keys {
		// key format: gateway:user:{user_id}:{device_id}
		parts := strings.Split(key, ":")
		if len(parts) >= 4 {
			devices = append(devices, parts[3])
		}
	}
	return devices, nil
}

func (s *Service) appendSyncEvent(ctx context.Context, userID int64, p struct {
	ServerMessageID string `json:"server_message_id"`
	ConversationID  string `json:"conversation_id"`
	ConversationSeq int64  `json:"conversation_seq"`
	SenderUserID    int64  `json:"sender_user_id"`
	SenderDeviceID  string `json:"sender_device_id"`
	MessageType     string `json:"message_type"`
	CreatedAt       string `json:"created_at"`
}) {
	payload := map[string]interface{}{
		"server_message_id": p.ServerMessageID,
		"conversation_id":   p.ConversationID,
		"conversation_seq":  p.ConversationSeq,
		"sender_user_id":    p.SenderUserID,
		"sender_device_id":  p.SenderDeviceID,
		"message_type":      p.MessageType,
	}
	payloadJSON, _ := json.Marshal(payload)
	if err := s.sync.AppendEvent(ctx, userID, domain.EventTypeMessageCreated, payloadJSON); err != nil {
		slog.Error("append sync event", "user_id", userID, "error", err)
	}
}

func redisUserPattern(userID int64) string {
	return "gateway:user:" + strconv.FormatInt(userID, 10) + ":*"
}
