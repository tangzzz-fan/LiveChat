package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/auth"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/messages"
)

// Router builds the HTTP handler tree for Message Service.
func NewRouter(db *sql.DB, rdb *redis.Client, authSvc *auth.Service) http.Handler {
	mux := http.NewServeMux()

	msgSvc := messages.NewService(db)

	// Auth endpoints (public)
	mux.HandleFunc("POST /v1/auth/register", handleRegister(db, authSvc))
	mux.HandleFunc("POST /v1/auth/login", handleLogin(db, authSvc))

	// Health
	mux.HandleFunc("GET /health", handleHealth(db, rdb))

	// Authenticated endpoints
	authMw := newAuthMiddleware(authSvc)
	mux.Handle("POST /v1/messages/send", authMw.Wrap(http.HandlerFunc(handleSendMessage(msgSvc))))

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

func errorResponse(msg string) map[string]interface{} {
	return map[string]interface{}{"error": msg}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}
