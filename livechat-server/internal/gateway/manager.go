package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/auth"
	livechat "github.com/tangzzz-fan/LiveChat/livechat-server/proto"
	"google.golang.org/protobuf/proto"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// Session represents an authenticated WebSocket connection.
type Session struct {
	ID          string
	UserID      int64
	DeviceID    string
	Conn        *websocket.Conn
	LastReadAt  time.Time
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex // guards writes
}

// Send writes a frame to the WebSocket connection (thread-safe).
func (s *Session) Send(opcode uint32, msg proto.Message) error {
	frame, err := NewFrame(opcode, msg)
	if err != nil {
		return err
	}
	raw, err := MarshalFrame(frame)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Conn.WriteMessage(websocket.BinaryMessage, raw)
}

// SendError sends an ErrorFrame to the connection.
func (s *Session) SendError(code uint32, message string, shouldReconnect bool) {
	_ = s.Send(OpError, &livechat.ErrorFrame{
		ErrorCode:       code,
		Message:         message,
		ShouldReconnect: shouldReconnect,
	})
}

// Close closes the WebSocket connection.
func (s *Session) Close() {
	s.cancel()
	s.Conn.Close()
}

// Manager tracks all active sessions and handles lifecycle.
type Manager struct {
	authSvc *auth.Service
	rdb     *redis.Client
	nodeID  string

	mu       sync.RWMutex
	sessions map[string]*Session // sessionID -> Session

	heartbeatInterval time.Duration
	readTimeout       time.Duration
}

func NewManager(authSvc *auth.Service, rdb *redis.Client, nodeID string, heartbeatInterval, readTimeout time.Duration) *Manager {
	return &Manager{
		authSvc:           authSvc,
		rdb:               rdb,
		nodeID:            nodeID,
		sessions:          make(map[string]*Session),
		heartbeatInterval: heartbeatInterval,
		readTimeout:       readTimeout,
	}
}

// HandleUpgrade performs WebSocket upgrade and handshake.
func (m *Manager) HandleUpgrade(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws upgrade failed", "error", err)
		return
	}

	// Read handshake request
	var raw []byte
	_, raw, err = conn.ReadMessage()
	if err != nil {
		slog.Error("read handshake frame", "error", err)
		conn.Close()
		return
	}

	frame, err := UnmarshalFrame(raw)
	if err != nil || frame.Opcode != OpHandshakeReq {
		// Send error
		errFrame, _ := NewFrame(OpError, &livechat.ErrorFrame{
			ErrorCode: 4000, Message: "expected HANDSHAKE_REQ",
			ShouldReconnect: false,
		})
		raw, _ := MarshalFrame(errFrame)
		conn.WriteMessage(websocket.BinaryMessage, raw)
		conn.Close()
		return
	}

	hsReq := &livechat.HandshakeRequest{}
	if len(frame.Payload) > 0 {
		proto.Unmarshal(frame.Payload, hsReq)
	}

	// Verify JWT
	claims, err := m.authSvc.VerifyAccessToken(hsReq.AccessToken)
	if err != nil {
		errFrame, _ := NewFrame(OpError, &livechat.ErrorFrame{
			ErrorCode: 4001, Message: "invalid or expired token",
			ShouldReconnect: false,
		})
		raw, _ := MarshalFrame(errFrame)
		conn.WriteMessage(websocket.BinaryMessage, raw)
		conn.Close()
		return
	}

	userID := claims.UserID
	deviceID := claims.DeviceID
	sessionID := generateSessionID(userID, deviceID)

	// Kick old session for same user+device
	m.mu.Lock()
	if old, ok := m.sessions[sessionID]; ok {
		old.SendError(4002, "new connection for this device", true)
		old.Close()
	}
	ctx, cancel := context.WithCancel(context.Background())
	sess := &Session{
		ID:         sessionID,
		UserID:     userID,
		DeviceID:   deviceID,
		Conn:       conn,
		LastReadAt: time.Now(),
		ctx:        ctx,
		cancel:     cancel,
	}
	m.sessions[sessionID] = sess
	m.mu.Unlock()

	// Register route in Redis
	m.registerRoute(ctx, userID, deviceID, sessionID)

	// Send handshake response
	resp := &livechat.HandshakeResponse{
		Success:             true,
		SessionId:           sessionID,
		NegotiatedVer:       ProtocolVersion,
		ServerTimeMs:        uint64(time.Now().UnixMilli()),
		HeartbeatIntervalS:  uint32(m.heartbeatInterval.Seconds()),
	}
	respFrame, _ := NewFrame(OpHandshakeResp, resp)
	raw, _ = MarshalFrame(respFrame)
	if err := conn.WriteMessage(websocket.BinaryMessage, raw); err != nil {
		slog.Error("send handshake resp", "error", err)
		m.removeSession(sessionID, userID, deviceID)
		return
	}

	slog.Info("session established", "session_id", sessionID, "user_id", userID, "device_id", deviceID)

	// Start read loop
	go m.readLoop(sess)
}

// readLoop reads frames from a session until disconnect.
func (m *Manager) readLoop(sess *Session) {
	defer func() {
		m.removeSession(sess.ID, sess.UserID, sess.DeviceID)
		sess.Close()
		slog.Info("session closed", "session_id", sess.ID)
	}()

	for {
		sess.Conn.SetReadDeadline(time.Now().Add(m.readTimeout))
		_, raw, err := sess.Conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				slog.Debug("read error", "session_id", sess.ID, "error", err)
			}
			return
		}
		sess.LastReadAt = time.Now()

		frame, err := UnmarshalFrame(raw)
		if err != nil {
			slog.Debug("unmarshal frame", "session_id", sess.ID, "error", err)
			continue
		}

		m.handleFrame(sess, frame)
	}
}

// handleFrame dispatches an incoming frame based on opcode.
func (m *Manager) handleFrame(sess *Session, frame *livechat.WsFrame) {
	switch frame.Opcode {
	case OpHeartbeat:
		// Reply with HEARTBEAT_ACK and refresh Redis TTL
		ack, _ := NewFrame(OpHeartbeatAck, &livechat.HeartbeatAck{
			ServerTimeMs: uint64(time.Now().UnixMilli()),
		})
		raw, _ := MarshalFrame(ack)
		sess.mu.Lock()
		sess.Conn.WriteMessage(websocket.BinaryMessage, raw)
		sess.mu.Unlock()
		m.refreshRoute(sess.ctx, sess.UserID, sess.DeviceID)

	case OpDisconnect:
		slog.Info("client disconnect", "session_id", sess.ID)
		sess.Close()

	case OpAck:
		// ACK frames are forwarded to MessageService (to be implemented in ticket 0006/0009)
		ackMsg := &livechat.MessageAck{}
		if len(frame.Payload) > 0 {
			proto.Unmarshal(frame.Payload, ackMsg)
		}
		slog.Debug("received ack",
			"session_id", sess.ID,
			"ack_type", ackMsg.AckType,
			"event_seq", ackMsg.EventSeq,
		)

	case OpReconnect:
		// Reconnect with existing session_id — handled as a normal handshake for now
		slog.Debug("reconnect request", "session_id", sess.ID)

	default:
		slog.Debug("unknown opcode", "session_id", sess.ID, "opcode", frame.Opcode)
	}
}

// ── Session management ───────────────────────────────

func (m *Manager) removeSession(sessionID string, userID int64, deviceID string) {
	m.mu.Lock()
	delete(m.sessions, sessionID)
	m.mu.Unlock()
	m.unregisterRoute(context.Background(), userID, deviceID)
}

// GetSession finds a session by user_id + device_id.
func (m *Manager) GetSession(userID int64, deviceID string) *Session {
	sid := makeSessionID(userID, deviceID)
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[sid]
}

// ActiveSessions returns the count of connected sessions.
func (m *Manager) ActiveSessions() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// ── Heartbeat watchdog ───────────────────────────────

// StartWatchdog starts a background goroutine that checks for stale connections.
func (m *Manager) StartWatchdog(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkStale()
		}
	}
}

func (m *Manager) checkStale() {
	now := time.Now()
	m.mu.RLock()
	stale := make([]*Session, 0)
	for _, sess := range m.sessions {
		if now.Sub(sess.LastReadAt) > m.readTimeout {
			stale = append(stale, sess)
		}
	}
	m.mu.RUnlock()

	for _, sess := range stale {
		slog.Info("closing stale session", "session_id", sess.ID, "user_id", sess.UserID)
		sess.SendError(4003, "connection timeout", true)
		sess.Close()
	}
}

// ── Redis routing ────────────────────────────────────

func (m *Manager) registerRoute(ctx context.Context, userID int64, deviceID, sessionID string) {
	key := redisUserKey(userID, deviceID)
	nodeKey := redisNodeKey(m.nodeID)
	val := fmt.Sprintf("%s:%s", m.nodeID, sessionID)

	pipe := m.rdb.Pipeline()
	pipe.Set(ctx, key, val, 300*time.Second) // 5 min TTL
	pipe.SAdd(ctx, nodeKey, fmt.Sprintf("%d:%s", userID, deviceID))
	pipe.Expire(ctx, nodeKey, 300*time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		slog.Error("register route", "error", err)
	}
}

func (m *Manager) refreshRoute(ctx context.Context, userID int64, deviceID string) {
	key := redisUserKey(userID, deviceID)
	m.rdb.Expire(ctx, key, 300*time.Second)
}

func (m *Manager) unregisterRoute(ctx context.Context, userID int64, deviceID string) {
	key := redisUserKey(userID, deviceID)
	nodeKey := redisNodeKey(m.nodeID)
	pipe := m.rdb.Pipeline()
	pipe.Del(ctx, key)
	pipe.SRem(ctx, nodeKey, fmt.Sprintf("%d:%s", userID, deviceID))
	if _, err := pipe.Exec(ctx); err != nil {
		slog.Error("unregister route", "error", err)
	}
}

func redisUserKey(userID int64, deviceID string) string {
	return fmt.Sprintf("gateway:user:%d:%s", userID, deviceID)
}

func redisNodeKey(nodeID string) string {
	return fmt.Sprintf("gateway:node:%s:connections", nodeID)
}

// ── Helpers ──────────────────────────────────────────

func makeSessionID(userID int64, deviceID string) string {
	return fmt.Sprintf("%d:%s", userID, deviceID)
}

func generateSessionID(userID int64, deviceID string) string {
	return makeSessionID(userID, deviceID)
}
