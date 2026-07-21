package domain

import "time"

// User represents a registered account.
type User struct {
	ID          int64     `json:"id"`
	PhoneE164   string    `json:"phone_e164"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
}

// Device represents a logged-in device instance.
type Device struct {
	ID               string    `json:"id"`
	UserID           int64     `json:"user_id"`
	Platform         string    `json:"platform"`
	PushToken        string    `json:"push_token,omitempty"`
	RefreshTokenHash string    `json:"-"`
	SessionVersion   int       `json:"session_version"`
	LastSeenAt       time.Time `json:"last_seen_at"`
	CreatedAt        time.Time `json:"created_at"`
}

// Conversation is the logical container for messages.
type Conversation struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"` // "direct" or "group"
	CreatedAt time.Time `json:"created_at"`
}

// ConversationMember links a user to a conversation.
type ConversationMember struct {
	ConversationID string    `json:"conversation_id"`
	UserID         int64     `json:"user_id"`
	JoinedAt       time.Time `json:"joined_at"`
}

// Message is a single business message within a conversation.
type Message struct {
	ServerMessageID  string    `json:"server_message_id"`
	ConversationID   string    `json:"conversation_id"`
	ConversationSeq  int64     `json:"conversation_seq"`
	SenderUserID     int64     `json:"sender_user_id"`
	SenderDeviceID   string    `json:"sender_device_id"`
	ClientMessageID  string    `json:"client_message_id"`
	MessageType      string    `json:"message_type"`
	Content          string    `json:"content"` // JSON-encoded payload
	ServerReceivedAt time.Time `json:"server_received_at"`
}

// MessageReceipt records delivery or read status.
type MessageReceipt struct {
	ServerMessageID string    `json:"server_message_id"`
	UserID          int64     `json:"user_id"`
	DeviceID        string    `json:"device_id"`
	ReceiptType     string    `json:"receipt_type"` // "delivered" or "read"
	AckedAt         time.Time `json:"acked_at"`
}

// ConversationSummary is a precomputed projection for the conversation list.
type ConversationSummary struct {
	UserID             int64                       `json:"user_id"`
	ConversationID     string                      `json:"conversation_id"`
	ConversationType   string                      `json:"conversation_type"`
	LastMessagePreview string                      `json:"last_message_preview"`
	LastMessageAt      time.Time                   `json:"last_message_at"`
	UnreadCount        int                         `json:"unread_count"`
	IsPinned           bool                        `json:"is_pinned"`
	Members            []ConversationSummaryMember `json:"members"`
	UpdatedAt          time.Time                   `json:"updated_at"`
}

type ConversationSummaryMember struct {
	UserID      int64  `json:"user_id"`
	DisplayName string `json:"display_name"`
}

// OutboxEvent is persisted in the outbox_events table.
type OutboxEvent struct {
	ID            int64      `json:"id"`
	AggregateType string     `json:"aggregate_type"`
	AggregateID   string     `json:"aggregate_id"`
	EventType     string     `json:"event_type"`
	Payload       string     `json:"payload"` // JSON-encoded
	Status        string     `json:"status"`
	RetryCount    int        `json:"retry_count"`
	LastError     string     `json:"last_error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	ProcessedAt   *time.Time `json:"processed_at,omitempty"`
}

// SyncEvent is an event in a user's incremental sync stream.
type SyncEvent struct {
	EventSeq       int64     `json:"event_seq"`
	UserID         int64     `json:"user_id"`
	ConversationID string    `json:"conversation_id,omitempty"`
	EventType      string    `json:"event_type"`
	Payload        string    `json:"payload"` // JSON-encoded
	CreatedAt      time.Time `json:"created_at"`
}

// SyncCursor tracks a device's consumed position in the sync stream.
type SyncCursor struct {
	UserID       int64     `json:"user_id"`
	DeviceID     string    `json:"device_id"`
	LastEventSeq int64     `json:"last_event_seq"`
	LastSyncAt   time.Time `json:"last_sync_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AuditEvent records a security-relevant event.
type AuditEvent struct {
	ID            int64     `json:"id"`
	UserID        int64     `json:"user_id"`
	DeviceID      string    `json:"device_id,omitempty"`
	EventType     string    `json:"event_type"`
	IPAddress     string    `json:"ip_address"`
	UserAgent     string    `json:"user_agent"`
	FailureReason string    `json:"failure_reason,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// ── Event type constants ────────────────────────────

const (
	EventTypeMessageCreated      = "message_created"
	EventTypeMessageDelivered    = "message_delivered"
	EventTypeMessageRead         = "message_read"
	EventTypeConversationUpdated = "conversation_updated"

	// Audit event types
	EventTypeLoginSuccess       = "login_success"
	EventTypeLoginFailed        = "login_failed"
	EventTypeCodeRequest        = "code_request"
	EventTypeDeviceAdded        = "device_added"
	EventTypeDeviceRevoked      = "device_revoked"
	EventTypeTokenRefreshed     = "token_refreshed"
	EventTypeTokenReplayDetected = "token_replay_detected"
	EventTypeSecurityAlert      = "security_alert"
)

// ── Outbox aggregate types ──────────────────────────

const (
	AggregateTypeMessage = "message"
	AggregateTypeReceipt = "receipt"
)

// ── Outbox statuses ─────────────────────────────────

const (
	OutboxStatusPending    = "pending"
	OutboxStatusProcessing = "processing"
	OutboxStatusDone       = "done"
	OutboxStatusFailed     = "failed"
)

// ── Receipt types ───────────────────────────────────

const (
	ReceiptTypeDelivered = "delivered"
	ReceiptTypeRead      = "read"
)

// ── Message types ───────────────────────────────────

const (
	MessageTypeText = "text"
)

// ── Conversation types ──────────────────────────────

const (
	ConversationTypeDirect = "direct"
	ConversationTypeGroup  = "group"
)
