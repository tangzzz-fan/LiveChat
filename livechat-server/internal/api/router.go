package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/auth"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/conversations"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/domain"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/group"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/media"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/messages"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/metrics"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/sync"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/traceutil"
)

// Router builds the HTTP handler tree for Message Service.
func NewRouter(db *sql.DB, rdb *redis.Client, authSvc *auth.Service, mediaSvc *media.Service) http.Handler {
	mux := http.NewServeMux()

	msgSvc := messages.NewService(db)
	syncSvc := sync.NewService(db)
	convSvc := conversations.NewService(db)
	groupSvc := group.NewService(db)
	groupSvc.SetSyncWriter(syncSvc)
	msgSvc.SetConversationUpdater(convSvc)

	// Auth endpoints (public)
	mux.HandleFunc("POST /v1/auth/register", handleRegister(db, authSvc))
	mux.HandleFunc("POST /v1/auth/login", handleLogin(db, authSvc))
	mux.HandleFunc("POST /v1/auth/refresh", handleRefreshToken(db, authSvc))

	// New auth flow (Spec 03 two-step)
	mux.HandleFunc("POST /v1/auth/request_code", handleRequestCode(rdb))
	mux.HandleFunc("POST /v1/auth/verify_code", handleVerifyCode(db, rdb, authSvc))

	// Health
	mux.HandleFunc("GET /health", handleHealth(db, rdb))

	// Metrics
	mux.Handle("GET /metrics", metrics.Handler())

	// Device management
	authMw := newAuthMiddleware(authSvc, db)
	mux.Handle("GET /v1/devices", authMw.Wrap(http.HandlerFunc(handleListDevices(db))))
	mux.Handle("POST /v1/devices/{did}/revoke", authMw.Wrap(http.HandlerFunc(handleRevokeDevice(db))))
	mux.Handle("POST /v1/devices/push-token", authMw.Wrap(http.HandlerFunc(handleUpdatePushToken(db))))

	// Core endpoints
	mux.Handle("POST /v1/messages/send", authMw.Wrap(http.HandlerFunc(handleSendMessage(msgSvc))))
	mux.Handle("GET /v1/sync/events", authMw.Wrap(http.HandlerFunc(handleGetSyncEvents(syncSvc))))
	mux.Handle("GET /v1/conversations/{cid}/messages", authMw.Wrap(http.HandlerFunc(handleGetMessages(syncSvc))))
	mux.Handle("GET /v1/conversations", authMw.Wrap(http.HandlerFunc(handleListConversations(convSvc))))

	// Group endpoints
	mux.Handle("POST /v1/groups", authMw.Wrap(http.HandlerFunc(handleCreateGroup(groupSvc))))
	mux.Handle("POST /v1/groups/{gid}/members", authMw.Wrap(http.HandlerFunc(handleAddGroupMembers(groupSvc))))
	mux.Handle("DELETE /v1/groups/{gid}/members/{uid}", authMw.Wrap(http.HandlerFunc(handleRemoveGroupMember(groupSvc))))
	mux.Handle("POST /v1/groups/{gid}/leave", authMw.Wrap(http.HandlerFunc(handleLeaveGroup(groupSvc))))
	mux.Handle("GET /v1/groups/{gid}/members", authMw.Wrap(http.HandlerFunc(handleListGroupMembers(groupSvc))))

	// Media endpoints (P0)
	if mediaSvc != nil {
		mux.Handle("POST /v1/media/upload/initiate", authMw.Wrap(http.HandlerFunc(handleMediaUploadInitiate(mediaSvc))))
		mux.Handle("GET /v1/media/upload/{uploadID}/status", authMw.Wrap(http.HandlerFunc(handleMediaUploadStatus(mediaSvc))))
		mux.Handle("POST /v1/media/upload/{uploadID}/complete", authMw.Wrap(http.HandlerFunc(handleMediaUploadComplete(mediaSvc))))
		mux.Handle("POST /v1/media/download/auth", authMw.Wrap(http.HandlerFunc(handleMediaDownloadAuth(mediaSvc))))
		// Part upload and download endpoints are public (presigned URLs carry their own auth)
		mux.HandleFunc("PUT /media/upload-part/", handleMediaUploadPart(mediaSvc))
		mux.HandleFunc("GET /media/download/", handleMediaDownload(mediaSvc))
	}

	return withLogging(securityHeaders(mux))
}

// ── Auth middleware ──────────────────────────────────

type authMiddleware struct {
	svc *auth.Service
	db  *sql.DB
}

func newAuthMiddleware(svc *auth.Service, db *sql.DB) *authMiddleware {
	return &authMiddleware{svc: svc, db: db}
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

		// Check session_version against DB to detect revoked devices
		var dbVersion int
		err = m.db.QueryRowContext(r.Context(),
			"SELECT session_version FROM devices WHERE id=$1 AND user_id=$2",
			claims.DeviceID, claims.UserID,
		).Scan(&dbVersion)
		if err != nil || int64(dbVersion) > claims.SessionVersion {
			writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
				"error":      "device has been revoked or session expired",
				"error_code": "device_revoked",
			})
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

// Deprecated: use POST /v1/auth/request_code + POST /v1/auth/verify_code instead.
// Kept for backward compatibility with Phase 1 clients and tests.
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

		var userID int64
		err := db.QueryRowContext(r.Context(),
			"INSERT INTO users (phone_e164, display_name) VALUES ($1, $2) RETURNING id",
			req.PhoneE164, req.PhoneE164).Scan(&userID)
		if err != nil {
			var pqErr *pq.Error
			if errors.As(err, &pqErr) && pqErr.Code == "23505" {
				writeJSON(w, http.StatusConflict, errorResponse("user already exists"))
				return
			}
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

		accessToken, err := authSvc.SignAccessToken(userID, req.DeviceID, 1)
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

// Deprecated: use POST /v1/auth/request_code + POST /v1/auth/verify_code instead.
// Kept for backward compatibility with Phase 1 clients and tests.
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

		accessToken, err := authSvc.SignAccessToken(userID, req.DeviceID, 1)
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

// ── New auth handlers ──────────────────────────────────

var e164RE = regexp.MustCompile(`^\+[1-9]\d{6,14}$`)

type requestCodeResponse struct {
	RetryAfterSec int `json:"retry_after_sec"`
	ExpiresInSec  int `json:"expires_in_sec"`
}

func handleRequestCode(rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			PhoneE164 string `json:"phone_e164"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid request body"))
			return
		}
		if !e164RE.MatchString(req.PhoneE164) {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid phone_e164 format"))
			return
		}

		ctx := r.Context()

		// Per-phone rate limit: 3/hour
		phoneKey := "rate:phone:" + req.PhoneE164
		phoneCount, err := rdb.Incr(ctx, phoneKey).Result()
		if err != nil {
			slog.Error("redis incr phone rate", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("internal error"))
			return
		}
		if phoneCount == 1 {
			rdb.Expire(ctx, phoneKey, time.Hour)
		}
		if phoneCount > 3 {
			writeJSON(w, http.StatusTooManyRequests, errorResponse("too many code requests for this phone number, try again later"))
			return
		}

		// Per-IP rate limit: 20/hour
		ip := r.RemoteAddr
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			ip = fwd
		}
		ipKey := "rate:ip:" + ip
		ipCount, err := rdb.Incr(ctx, ipKey).Result()
		if err != nil {
			slog.Error("redis incr ip rate", "error", err)
		}
		if ipCount == 1 {
			rdb.Expire(ctx, ipKey, time.Hour)
		}
		if ipCount > 20 {
			writeJSON(w, http.StatusTooManyRequests, errorResponse("too many code requests from this IP, try again later"))
			return
		}

		// Store mock verification code (always "123456")
		codeKey := "code:" + req.PhoneE164
		if err := rdb.Set(ctx, codeKey, "123456", 5*time.Minute).Err(); err != nil {
			slog.Error("redis set code", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("internal error"))
			return
		}

		writeJSON(w, http.StatusOK, requestCodeResponse{
			RetryAfterSec: 30,
			ExpiresInSec:  300,
		})
	}
}

type verifyCodeRequest struct {
	PhoneE164        string `json:"phone_e164"`
	VerificationCode string `json:"verification_code"`
	DeviceID         string `json:"device_id"`
	Platform         string `json:"platform"`
}

func handleVerifyCode(db *sql.DB, rdb *redis.Client, authSvc *auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req verifyCodeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid request body"))
			return
		}
		if !e164RE.MatchString(req.PhoneE164) {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid phone_e164 format"))
			return
		}
		if len(req.VerificationCode) != 6 {
			writeJSON(w, http.StatusBadRequest, errorResponse("verification_code must be 6 digits"))
			return
		}
		if req.DeviceID == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse("device_id is required"))
			return
		}

		ctx := r.Context()

		// Verify code from Redis
		codeKey := "code:" + req.PhoneE164
		storedCode, err := rdb.Get(ctx, codeKey).Result()
		if err == redis.Nil {
			writeJSON(w, http.StatusUnauthorized, errorResponse("verification code expired or not requested"))
			return
		}
		if err != nil {
			slog.Error("redis get code", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("internal error"))
			return
		}
		// Mock: accept any 6-digit code (storedCode is always "123456")
		_ = storedCode

		// One-time use: delete the code
		rdb.Del(ctx, codeKey)

		// Upsert user
		var userID int64
		err = db.QueryRowContext(ctx,
			`INSERT INTO users (phone_e164, display_name) VALUES ($1, $2)
			 ON CONFLICT (phone_e164) DO UPDATE SET display_name = EXCLUDED.display_name
			 RETURNING id`,
			req.PhoneE164, req.PhoneE164,
		).Scan(&userID)
		if err != nil {
			slog.Error("upsert user", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("failed to create user"))
			return
		}
		// Check if this device exists for this user
		var existingVersion int
		_ = db.QueryRowContext(ctx,
			"SELECT session_version FROM devices WHERE id=$1 AND user_id=$2",
			req.DeviceID, userID,
		).Scan(&existingVersion)
		isNewDevice := existingVersion == 0

		// Handle device_id collision: if same device_id registered by different user
		if isNewDevice {
			db.ExecContext(ctx, "DELETE FROM devices WHERE id=$1 AND user_id != $2", req.DeviceID, userID)
		}

		// Begin transaction for device upsert + audit
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			slog.Error("begin tx", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("internal error"))
			return
		}
		defer tx.Rollback()

		var sessionVersion int
		if isNewDevice {
			// New device: insert with session_version=1
			_, err = tx.ExecContext(ctx,
				`INSERT INTO devices (id, user_id, platform, session_version, last_seen_at)
				 VALUES ($1, $2, $3, 1, NOW())`,
				req.DeviceID, userID, req.Platform,
			)
			sessionVersion = 1
		} else {
			// Existing device: bump session_version
			err = tx.QueryRowContext(ctx,
				`UPDATE devices SET platform=$1, last_seen_at=NOW(), session_version=session_version+1
				 WHERE id=$2 AND user_id=$3
				 RETURNING session_version`,
				req.Platform, req.DeviceID, userID,
			).Scan(&sessionVersion)
		}
		if err != nil {
			slog.Error("upsert device in verify_code", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("failed to register device"))
			return
		}

		// Generate refresh token and update hash
		refreshRaw, refreshHash, err := auth.GenerateRefreshToken()
		if err != nil {
			slog.Error("generate refresh token", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("failed to generate token"))
			return
		}
		_, err = tx.ExecContext(ctx,
			"UPDATE devices SET refresh_token_hash=$1 WHERE id=$2 AND user_id=$3",
			refreshHash, req.DeviceID, userID,
		)
		if err != nil {
			slog.Error("update refresh token hash", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("failed to update device"))
			return
		}

		// Audit: login_success
		ip := r.RemoteAddr
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			ip = fwd
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO login_audit_events (user_id, device_id, event_type, ip_address, user_agent)
			 VALUES ($1, $2, $3, $4, $5)`,
			userID, req.DeviceID, domain.EventTypeLoginSuccess, ip, r.UserAgent(),
		)
		if err != nil {
			slog.Error("insert audit event", "error", err)
			// Non-fatal
		}

		// Audit: device_added if new device
		if isNewDevice {
			tx.ExecContext(ctx,
				`INSERT INTO login_audit_events (user_id, device_id, event_type, ip_address, user_agent)
				 VALUES ($1, $2, $3, $4, $5)`,
				userID, req.DeviceID, domain.EventTypeDeviceAdded, ip, r.UserAgent(),
			)
		}

		if err := tx.Commit(); err != nil {
			slog.Error("commit verify_code", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("internal error"))
			return
		}

		// Sign access token with the current session_version
		accessToken, err := authSvc.SignAccessToken(userID, req.DeviceID, sessionVersion)
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

		// Validate image message: attachment metadata is required
		if req.MessageType == domain.MessageTypeImage {
			var content map[string]interface{}
			if err := json.Unmarshal([]byte(req.Content), &content); err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse("invalid content JSON for image message"))
				return
			}
			att, ok := content["attachment"].(map[string]interface{})
			if !ok {
				writeJSON(w, http.StatusBadRequest, errorResponse("image messages require an 'attachment' field in content"))
				return
			}
			if att["object_key"] == nil || att["mime_type"] == nil || att["size_bytes"] == nil {
				writeJSON(w, http.StatusBadRequest, errorResponse("image attachment requires object_key, mime_type, and size_bytes"))
				return
			}
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
			TraceID:         TraceIDFromContext(r.Context()),
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

		metrics.MessagesSentTotal.Add(1)
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
	return traceutil.WithTraceID(ctx, traceID)
}

func TraceIDFromContext(ctx context.Context) string {
	return traceutil.TraceIDFromContext(ctx)
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

		// Issue new tokens (rotation) and bump session_version
		var sessionVersion int
		err = db.QueryRowContext(r.Context(),
			`UPDATE devices SET refresh_token_hash=$1, session_version=session_version+1, last_seen_at=NOW()
			 WHERE id=$2
			 RETURNING session_version`,
			"", deviceID, // placeholder — will update below
		).Scan(&sessionVersion)
		// Actually do a proper update:
		newRaw, newHash, _ := auth.GenerateRefreshToken()
		err = db.QueryRowContext(r.Context(),
			`UPDATE devices SET refresh_token_hash=$1, session_version=session_version+1, last_seen_at=NOW()
			 WHERE id=$2 AND user_id=$3
			 RETURNING session_version`,
			newHash, deviceID, userID,
		).Scan(&sessionVersion)
		if err != nil {
			slog.Error("update refresh token", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("internal error"))
			return
		}

		accessToken, _ := authSvc.SignAccessToken(userID, deviceID, sessionVersion)

		// Audit
		ip := r.RemoteAddr
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			ip = fwd
		}
		db.ExecContext(r.Context(),
			`INSERT INTO login_audit_events (user_id, device_id, event_type, ip_address, user_agent)
			 VALUES ($1, $2, $3, $4, $5)`,
			userID, deviceID, domain.EventTypeTokenRefreshed, ip, r.UserAgent(),
		)

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
			"SELECT id, platform, session_version, last_seen_at FROM devices WHERE user_id=$1 ORDER BY last_seen_at DESC",
			userID)
		if err != nil {
			slog.Error("list devices", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("internal error"))
			return
		}
		defer rows.Close()

		type deviceInfo struct {
			DeviceID       string    `json:"device_id"`
			Platform       string    `json:"platform"`
			SessionVersion int       `json:"session_version"`
			IsCurrent      bool      `json:"is_current"`
			LastSeenAt     time.Time `json:"last_seen_at"`
		}
		var devices []deviceInfo
		for rows.Next() {
			var d deviceInfo
			var lastSeen time.Time
			if err := rows.Scan(&d.DeviceID, &d.Platform, &d.SessionVersion, &lastSeen); err != nil {
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

		// Increment session_version so the device's existing JWT becomes invalid
		result, err := db.ExecContext(r.Context(),
			"UPDATE devices SET session_version = session_version + 1 WHERE id=$1 AND user_id=$2",
			did, userID)
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

		// Audit
		ip := r.RemoteAddr
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			ip = fwd
		}
		db.ExecContext(r.Context(),
			`INSERT INTO login_audit_events (user_id, device_id, event_type, ip_address, user_agent)
			 VALUES ($1, $2, $3, $4, $5)`,
			userID, did, domain.EventTypeDeviceRevoked, ip, r.UserAgent(),
		)

		writeJSON(w, http.StatusOK, map[string]interface{}{"revoked": did})
	}
}

func handleUpdatePushToken(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := UserIDFromContext(r.Context())
		deviceID := DeviceIDFromContext(r.Context())

		var req struct {
			PushToken string `json:"push_token"`
			Platform  string `json:"platform"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid request body"))
			return
		}
		if req.PushToken == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse("push_token is required"))
			return
		}

		setClauses := []string{"push_token = $1", "last_seen_at = NOW()"}
		args := []interface{}{req.PushToken}
		if req.Platform != "" {
			setClauses = append(setClauses, fmt.Sprintf("platform = $%d", len(args)+1))
			args = append(args, req.Platform)
		}
		args = append(args, deviceID, userID)

		query := fmt.Sprintf("UPDATE devices SET %s WHERE id = $%d AND user_id = $%d",
			strings.Join(setClauses, ", "), len(args)-1, len(args))
		_, err := db.ExecContext(r.Context(), query, args...)
		if err != nil {
			slog.Error("update push token", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("internal error"))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{"updated": true})
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

func handleLeaveGroup(svc *group.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := UserIDFromContext(r.Context())
		gid := r.PathValue("gid")

		if err := svc.LeaveGroup(r.Context(), gid, userID); err != nil {
			switch {
			case errors.Is(err, group.ErrNotMember):
				writeJSON(w, http.StatusNotFound, errorResponse("not a member of this group"))
			case errors.Is(err, group.ErrCannotRemoveSelf):
				writeJSON(w, http.StatusConflict, errorResponse("owner cannot leave; transfer ownership first"))
			default:
				slog.Error("leave group", "error", err)
				writeJSON(w, http.StatusInternalServerError, errorResponse("failed to leave group"))
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"left": gid})
	}
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		traceID := r.Header.Get("X-Trace-Id")
		if traceID == "" {
			traceID = generateTraceID()
		}
		w.Header().Set("X-Trace-Id", traceID)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r.WithContext(WithTraceID(r.Context(), traceID)))
		metrics.ObserveHTTPRequest(r.Method, r.URL.Path, rec.status, time.Since(start))
		slog.Info("request",
			"trace_id", traceID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func generateTraceID() string {
	sum := sha256.Sum256([]byte(traceutil.Generate() + "|" + strconv.FormatInt(time.Now().UnixNano(), 10)))
	return hex.EncodeToString(sum[:8])
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// ── Security middleware ─────────────────────────────

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

// ── Media handlers ─────────────────────────────────

func handleMediaUploadInitiate(svc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := UserIDFromContext(r.Context())
		var req media.UploadInitiateReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid request body"))
			return
		}
		resp, err := svc.InitiateUpload(r.Context(), userID, req)
		if err != nil {
			slog.Error("initiate upload", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse(err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func handleMediaDownloadAuth(svc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := UserIDFromContext(r.Context())
		var req media.DownloadAuthReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid request body"))
			return
		}
		resp, err := svc.AuthorizeDownload(r.Context(), userID, req)
		if err != nil {
			slog.Error("authorize download", "error", err)
			writeJSON(w, http.StatusForbidden, errorResponse("not authorized"))
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func handleMediaUploadStatus(svc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uploadID := r.PathValue("uploadID")
		if uploadID == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse("uploadID is required"))
			return
		}
		resp, err := svc.UploadStatus(r.Context(), uploadID)
		if err != nil {
			slog.Error("upload status", "error", err)
			writeJSON(w, http.StatusNotFound, errorResponse("upload not found"))
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func handleMediaUploadComplete(svc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uploadID := r.PathValue("uploadID")
		var req media.UploadCompleteReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid request body"))
			return
		}
		if req.ObjectKey == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse("object_key is required"))
			return
		}
		if err := svc.CompleteUpload(r.Context(), req.ObjectKey, uploadID, req.Parts); err != nil {
			slog.Error("complete upload", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse("failed to complete upload"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "completed"})
	}
}

// handleMediaUploadPart serves PUT /media/upload-part/{uploadID}/{partNumber}?exp=...&sig=...
// This is a public endpoint — auth is via presigned URL HMAC signature.
func handleMediaUploadPart(svc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			writeJSON(w, http.StatusMethodNotAllowed, errorResponse("method not allowed"))
			return
		}
		if err := svc.ServeUploadPart(r); err != nil {
			slog.Error("upload part", "error", err)
			writeJSON(w, http.StatusForbidden, errorResponse(err.Error()))
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

// handleMediaDownload serves GET /media/download/{key}?exp=...&sig=...
// This is a public endpoint — auth is via presigned URL HMAC signature.
func handleMediaDownload(svc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, errorResponse("method not allowed"))
			return
		}
		if err := svc.ServeDownload(w, r); err != nil {
			slog.Error("media download", "error", err)
			writeJSON(w, http.StatusForbidden, errorResponse(err.Error()))
			return
		}
	}
}
