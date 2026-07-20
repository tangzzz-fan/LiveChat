package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/auth"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/conversations"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/domain"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/group"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/messages"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/metrics"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/sync"
)

// Router builds the HTTP handler tree for Message Service.
func NewRouter(db *sql.DB, rdb *redis.Client, authSvc *auth.Service) http.Handler {
	mux := http.NewServeMux()

	msgSvc := messages.NewService(db)
	syncSvc := sync.NewService(db)
	convSvc := conversations.NewService(db)
	groupSvc := group.NewService(db)
	msgSvc.SetConversationUpdater(convSvc)

	// Auth endpoints (public)
	mux.HandleFunc("POST /v1/auth/register", handleRegister(db, authSvc))
	mux.HandleFunc("POST /v1/auth/login", handleLogin(db, authSvc))
	mux.HandleFunc("POST /v1/auth/refresh", handleRefreshToken(db, authSvc))

	// Health
	mux.HandleFunc("GET /health", handleHealth(db, rdb))

	// Metrics
	mux.Handle("GET /metrics", metrics.Handler())

	// Device management
	authMw := newAuthMiddleware(authSvc)
	mux.Handle("GET /v1/devices", authMw.Wrap(http.HandlerFunc(handleListDevices(db))))
	mux.Handle("POST /v1/devices/{did}/revoke", authMw.Wrap(http.HandlerFunc(handleRevokeDevice(db))))

	// Core endpoints
	mux.Handle("POST /v1/messages/send", authMw.Wrap(http.HandlerFunc(handleSendMessage(msgSvc))))
	mux.Handle("GET /v1/sync/events", authMw.Wrap(http.HandlerFunc(handleGetSyncEvents(syncSvc))))
	mux.Handle("GET /v1/conversations/{cid}/messages", authMw.Wrap(http.HandlerFunc(handleGetMessages(syncSvc))))
	mux.Handle("GET /v1/conversations", authMw.Wrap(http.HandlerFunc(handleListConversations(convSvc))))

	// Group endpoints
	mux.Handle("POST /v1/groups", authMw.Wrap(http.HandlerFunc(handleCreateGroup(groupSvc))))
	mux.Handle("POST /v1/groups/{gid}/members", authMw.Wrap(http.HandlerFunc(handleAddGroupMembers(groupSvc))))
	mux.Handle("DELETE /v1/groups/{gid}/members/{uid}", authMw.Wrap(http.HandlerFunc(handleRemoveGroupMember(groupSvc))))
	mux.Handle("GET /v1/groups/{gid}/members", authMw.Wrap(http.HandlerFunc(handleListGroupMembers(groupSvc))))

	return withLogging(mux)
}

// ── Auth middleware ──────────────────────────────────

type authMiddleware struct {
	svc *auth.Service
}

func newAuthMiddleware(svc *auth.Service) *authMiddleware {
	return &authMiddleware{svc: svc}
}

func (m *authMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if len(token) < 8 || token[:7] != "Bearer " {
			writeJSON(w, http.StatusUnauthorized, errorResponse("missing or malformed Authorization header"))
			return
		}
		claims, err := m.svc.VerifyAccessToken(token[7:])
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, errorResponse("invalid or expired token"))
			return
		}
		ctx := WithUserID(r.Context(), claims.UserID)
		ctx = WithDeviceID(ctx, claims.DeviceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ── Health ──────────────────────────────────────────

func handleHealth(db *sql.DB, rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		healthy := true
		details := map[string]string{}

		if err := db.PingContext(r.Context()); err != nil {
			healthy = false
			details["postgres"] = "error: " + err.Error()
		} else {
			details["postgres"] = "ok"
		}

		if err := rdb.Ping(r.Context()).Err(); err != nil {
			healthy = false
			details["redis"] = "error: " + err.Error()
		} else {
			details["redis"] = "ok"
		}

		status := http.StatusOK
		if !healthy {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, map[string]interface{}{
			"status":  map[bool]string{true: "ok", false: "degraded"}[healthy],
			"details": details,
		})
	}
}

// ── Auth handlers ────────────────────────────────────

type registerRequest struct {
	PhoneE164        string `json:"phone_e164"`
	VerificationCode string `json:"verification_code"`
	DeviceID         string `json:"device_id"`
	Platform         string `json:"platform"`
}

type authResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	UserID       int64  `json:"user_id"`
}

func handleRegister(db *sql.DB, authSvc *auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid request body"))
			return
		}
		if req.PhoneE164 == "" || req.DeviceID == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse("phone_e164 and device_id are required"))
			return
		}

		// Mock: accept any 6-digit code
		if len(req.VerificationCode) == 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse("verification_code is required"))
			return
		}

		// Check if user already exists
		var userID int64
		err := db.QueryRowContext(r.Context(),
			"INSERT INTO users (phone_e164, display_name) VALUES ($1, $2) ON CONFLICT (phone_e164) DO UPDATE SET phone_e164=EXCLUDED.phone_e164 RETURNING id",
			req.PhoneE164, req.PhoneE164).Scan(&userID)
		if err != nil {
			slog.Error("upsert user", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("failed to create user"))
			return
		}

		// upsert or insert device...
		// For mock, just insert if not exists
		refreshRaw, refreshHash, err := auth.GenerateRefreshToken()
		if err != nil {
			slog.Error("generate refresh token", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("failed to generate token"))
			return
		}

		_, err = db.ExecContext(r.Context(),
			`INSERT INTO devices (id, user_id, platform, refresh_token_hash, last_seen_at)
			 VALUES ($1, $2, $3, $4, NOW())
			 ON CONFLICT (id) DO UPDATE SET refresh_token_hash=EXCLUDED.refresh_token_hash, last_seen_at=NOW()`,
			req.DeviceID, userID, req.Platform, refreshHash)
		if err != nil {
			slog.Error("upsert device", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("failed to register device"))
			return
		}

		accessToken, err := authSvc.SignAccessToken(userID, req.DeviceID)
		if err != nil {
			slog.Error("sign access token", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("failed to sign token"))
			return
		}

		writeJSON(w, http.StatusCreated, authResponse{
			AccessToken:  accessToken,
			RefreshToken: refreshRaw,
			ExpiresIn:    3600,
			UserID:       userID,
		})
	}
}

func handleLogin(db *sql.DB, authSvc *auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid request body"))
			return
		}
		if req.PhoneE164 == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse("phone_e164 is required"))
			return
		}
		if len(req.VerificationCode) == 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse("verification_code is required"))
			return
		}

		var userID int64
		err := db.QueryRowContext(r.Context(),
			"SELECT id FROM users WHERE phone_e164=$1", req.PhoneE164).Scan(&userID)
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusUnauthorized, errorResponse("user not found"))
			return
		}
		if err != nil {
			slog.Error("lookup user", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("internal error"))
			return
		}

		// Upsert device
		refreshRaw, refreshHash, err := auth.GenerateRefreshToken()
		if err != nil {
			slog.Error("generate refresh token", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("failed to generate token"))
			return
		}

		_, err = db.ExecContext(r.Context(),
			`INSERT INTO devices (id, user_id, platform, refresh_token_hash, last_seen_at)
			 VALUES ($1, $2, $3, $4, NOW())
			 ON CONFLICT (id) DO UPDATE SET refresh_token_hash=EXCLUDED.refresh_token_hash, last_seen_at=NOW()`,
			req.DeviceID, userID, req.Platform, refreshHash)
		if err != nil {
			slog.Error("upsert device", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("failed to update device"))
			return
		}

		accessToken, err := authSvc.SignAccessToken(userID, req.DeviceID)
		if err != nil {
			slog.Error("sign access token", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("failed to sign token"))
			return
		}

		writeJSON(w, http.StatusOK, authResponse{
			AccessToken:  accessToken,
			RefreshToken: refreshRaw,
			ExpiresIn:    3600,
			UserID:       userID,
		})
	}
}

// ── Message send handler ────────────────────────────

type sendMessageRequest struct {
	ClientMessageID string `json:"client_message_id"`
	ConversationID  string `json:"conversation_id"`
	MessageType     string `json:"message_type"`
	Content         string `json:"content"`
}

func handleSendMessage(svc *messages.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req sendMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid request body"))
			return
		}

		if req.ClientMessageID == "" || req.ConversationID == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse("client_message_id and conversation_id are required"))
			return
		}
		if req.MessageType == "" {
			req.MessageType = "text"
		}

		userID := UserIDFromContext(r.Context())
		deviceID := DeviceIDFromContext(r.Context())

		result, err := svc.Send(r.Context(), messages.SendRequest{
			ClientMessageID: req.ClientMessageID,
			ConversationID:  req.ConversationID,
			SenderUserID:    userID,
			SenderDeviceID:  deviceID,
			MessageType:     req.MessageType,
			Content:         req.Content,
		})
		if err != nil {
			if errors.Is(err, messages.ErrNotMember) {
				writeJSON(w, http.StatusForbidden, errorResponse("you are not a member of this conversation"))
				return
			}
			slog.Error("send message", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("failed to send message"))
			return
		}

		writeJSON(w, http.StatusOK, result)
	}
}

// ── Sync events handler ──────────────────────────────

func handleGetSyncEvents(svc *sync.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := UserIDFromContext(r.Context())
		deviceID := DeviceIDFromContext(r.Context())

		cursorStr := r.URL.Query().Get("cursor")
		limitStr := r.URL.Query().Get("limit")
		cursor := int64(0)
		if cursorStr != "" {
			var err error
			cursor, err = strconv.ParseInt(cursorStr, 10, 64)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse("invalid cursor"))
				return
			}
		}
		limit := 100
		if limitStr != "" {
			if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}

		events, latestSeq, err := svc.GetEvents(r.Context(), userID, cursor, limit)
		if err != nil {
			slog.Error("get sync events", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("internal error"))
			return
		}

		// Update cursor after sync
		if len(events) > 0 {
			lastEventSeq := events[len(events)-1].EventSeq
			if err := svc.UpdateCursor(r.Context(), userID, deviceID, lastEventSeq); err != nil {
				slog.Error("update cursor", "error", err)
				// non-fatal
			}
		}

		hasMore := len(events) >= limit
		if events == nil {
			events = make([]domain.SyncEvent, 0)
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"events":           events,
			"has_more":         hasMore,
			"latest_event_seq": latestSeq,
			"server_time_ms":   time.Now().UnixMilli(),
		})
	}
}

// ── Conversation messages handler ────────────────────

func handleGetMessages(svc *sync.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := UserIDFromContext(r.Context())
		cid := r.PathValue("cid")

		fromSeqStr := r.URL.Query().Get("from_seq")
		limitStr := r.URL.Query().Get("limit")

		fromSeq := int64(0)
		if fromSeqStr != "" {
			var err error
			fromSeq, err = strconv.ParseInt(fromSeqStr, 10, 64)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse("invalid from_seq"))
				return
			}
		}
		limit := 50
		if limitStr != "" {
			if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 100 {
				limit = n
			}
		}

		// Verify membership
		ok, err := svc.VerifyMembership(r.Context(), userID, cid)
		if err != nil {
			slog.Error("check membership", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("internal error"))
			return
		}
		if !ok {
			writeJSON(w, http.StatusForbidden, errorResponse("not a member of this conversation"))
			return
		}

		messages, err := svc.GetMessages(r.Context(), cid, fromSeq, limit)
		if err != nil {
			slog.Error("get messages", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("internal error"))
			return
		}
		if messages == nil {
			messages = make([]domain.Message, 0)
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"messages": messages,
		})
	}
}

// ── Conversation list handler ───────────────────────

func handleListConversations(svc *conversations.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := UserIDFromContext(r.Context())

		limitStr := r.URL.Query().Get("limit")
		offsetStr := r.URL.Query().Get("offset")

		limit := 50
		offset := 0
		if limitStr != "" {
			if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}
		if offsetStr != "" {
			if n, err := strconv.Atoi(offsetStr); err == nil && n >= 0 {
				offset = n
			}
		}

		summaries, err := svc.List(r.Context(), userID, limit, offset)
		if err != nil {
			slog.Error("list conversations", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("internal error"))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"conversations": summaries,
		})
	}
}

// ── Helpers ─────────────────────────────────────────

type contextKey string

const (
	ctxUserID   contextKey = "user_id"
	ctxDeviceID contextKey = "device_id"
)

func WithUserID(ctx context.Context, uid int64) context.Context {
	return context.WithValue(ctx, ctxUserID, uid)
}

func UserIDFromContext(ctx context.Context) int64 {
	v, _ := ctx.Value(ctxUserID).(int64)
	return v
}

func WithDeviceID(ctx context.Context, did string) context.Context {
	return context.WithValue(ctx, ctxDeviceID, did)
}

func DeviceIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxDeviceID).(string)
	return v
}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, contextKey("trace_id"), traceID)
}

func TraceIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(contextKey("trace_id")).(string)
	return v
}

func errorResponse(msg string) map[string]interface{} {
	return map[string]interface{}{"error": msg}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ── Refresh token handler ──────────────────────────

func handleRefreshToken(db *sql.DB, authSvc *auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RefreshToken string `json:"refresh_token"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.RefreshToken == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse("refresh_token is required"))
			return
		}

		// Hash the incoming token and look up device
		h := sha256.Sum256([]byte(req.RefreshToken))
		hash := hex.EncodeToString(h[:])
		var userID int64
		var deviceID string
		err := db.QueryRowContext(r.Context(),
			"SELECT user_id, id FROM devices WHERE refresh_token_hash=$1", hash,
		).Scan(&userID, &deviceID)
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusUnauthorized, errorResponse("invalid refresh token"))
			return
		}
		if err != nil {
			slog.Error("refresh token lookup", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("internal error"))
			return
		}

		// Issue new tokens (rotation)
		accessToken, _ := authSvc.SignAccessToken(userID, deviceID)
		newRaw, newHash, _ := auth.GenerateRefreshToken()

		db.ExecContext(r.Context(),
			"UPDATE devices SET refresh_token_hash=$1 WHERE id=$2", newHash, deviceID)

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"access_token":  accessToken,
			"refresh_token": newRaw,
			"expires_in":    3600,
		})
	}
}

// ── Device management handlers ─────────────────────

func handleListDevices(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := UserIDFromContext(r.Context())
		deviceID := DeviceIDFromContext(r.Context())

		rows, err := db.QueryContext(r.Context(),
			"SELECT id, platform, last_seen_at FROM devices WHERE user_id=$1 ORDER BY last_seen_at DESC",
			userID)
		if err != nil {
			slog.Error("list devices", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("internal error"))
			return
		}
		defer rows.Close()

		type deviceInfo struct {
			DeviceID   string    `json:"device_id"`
			Platform   string    `json:"platform"`
			IsCurrent  bool      `json:"is_current"`
			LastSeenAt time.Time `json:"last_seen_at"`
		}
		var devices []deviceInfo
		for rows.Next() {
			var d deviceInfo
			var lastSeen time.Time
			if err := rows.Scan(&d.DeviceID, &d.Platform, &lastSeen); err != nil {
				continue
			}
			d.LastSeenAt = lastSeen
			d.IsCurrent = d.DeviceID == deviceID
			devices = append(devices, d)
		}
		if devices == nil {
			devices = make([]deviceInfo, 0)
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"devices": devices})
	}
}

func handleRevokeDevice(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := UserIDFromContext(r.Context())
		did := r.PathValue("did")

		result, err := db.ExecContext(r.Context(),
			"DELETE FROM devices WHERE id=$1 AND user_id=$2", did, userID)
		if err != nil {
			slog.Error("revoke device", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("internal error"))
			return
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			writeJSON(w, http.StatusNotFound, errorResponse("device not found"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"revoked": did})
	}
}

// ── Group handlers ─────────────────────────────────

func handleCreateGroup(svc *group.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := UserIDFromContext(r.Context())

		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Name == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse("name is required"))
			return
		}

		g, err := svc.CreateGroup(r.Context(), req.Name, req.Description, userID)
		if err != nil {
			slog.Error("create group", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("failed to create group"))
			return
		}

		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"group":           g,
			"conversation_id": svc.GetConversationID(g.ID),
		})
	}
}

func handleAddGroupMembers(svc *group.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := UserIDFromContext(r.Context())
		gid := r.PathValue("gid")

		var req struct {
			UserIDs []int64 `json:"user_ids"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.UserIDs) == 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse("user_ids is required"))
			return
		}

		if err := svc.AddMembers(r.Context(), gid, userID, req.UserIDs); err != nil {
			switch {
			case errors.Is(err, group.ErrNotAdmin):
				writeJSON(w, http.StatusForbidden, errorResponse("requires admin privileges"))
			case errors.Is(err, group.ErrGroupFull):
				writeJSON(w, http.StatusConflict, errorResponse("group is full"))
			case errors.Is(err, group.ErrAlreadyMember):
				writeJSON(w, http.StatusConflict, errorResponse("user already a member"))
			default:
				slog.Error("add members", "error", err)
				writeJSON(w, http.StatusInternalServerError, errorResponse("failed to add members"))
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"added": len(req.UserIDs)})
	}
}

func handleRemoveGroupMember(svc *group.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := UserIDFromContext(r.Context())
		gid := r.PathValue("gid")
		uidStr := r.PathValue("uid")
		targetUID, _ := strconv.ParseInt(uidStr, 10, 64)

		if err := svc.RemoveMember(r.Context(), gid, userID, targetUID); err != nil {
			switch {
			case errors.Is(err, group.ErrNotAdmin):
				writeJSON(w, http.StatusForbidden, errorResponse("requires admin privileges"))
			case errors.Is(err, group.ErrCannotRemoveOwner):
				writeJSON(w, http.StatusConflict, errorResponse("cannot remove the group owner"))
			case errors.Is(err, group.ErrNotMember):
				writeJSON(w, http.StatusNotFound, errorResponse("user is not a member"))
			default:
				slog.Error("remove member", "error", err)
				writeJSON(w, http.StatusInternalServerError, errorResponse("failed to remove member"))
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"removed": targetUID})
	}
}

func handleListGroupMembers(svc *group.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gid := r.PathValue("gid")
		members, err := svc.GetMembers(r.Context(), gid)
		if err != nil {
			slog.Error("list members", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("internal error"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"members": members})
	}
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		traceID := r.Header.Get("X-Trace-Id")
		if traceID == "" {
			traceID = generateTraceID(r)
		}
		w.Header().Set("X-Trace-Id", traceID)
		next.ServeHTTP(w, r.WithContext(WithTraceID(r.Context(), traceID)))
		slog.Info("request",
			"trace_id", traceID,
			"method", r.Method,
			"path", r.URL.Path,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func generateTraceID(r *http.Request) string {
	sum := sha256.Sum256([]byte(r.Method + "|" + r.URL.Path + "|" + strconv.FormatInt(time.Now().UnixNano(), 10)))
	return hex.EncodeToString(sum[:8])
}
