package media

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/gif"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/image/draw"
)

// ObjectStore is the interface for storing and retrieving media objects.
type ObjectStore interface {
	InitiateMultipartUpload(ctx context.Context, key string, contentType string) (uploadID string, err error)
	PresignUploadPart(ctx context.Context, key, uploadID string, partNumber int, expires time.Duration) (url string, err error)
	CompleteMultipartUpload(ctx context.Context, key, uploadID string, parts []Part) error
	ListCompletedParts(ctx context.Context, key, uploadID string) ([]Part, error)
	GetObject(ctx context.Context, key string) ([]byte, error)
	PutObject(ctx context.Context, key string, data []byte, contentType string) error
	PresignDownload(ctx context.Context, key string, expires time.Duration) (url string, err error)
	CleanupUpload(ctx context.Context, key, uploadID string) error
}

type Part struct {
	PartNumber int    `json:"part_number"`
	ETag       string `json:"etag"`
}

// ── Local file system implementation ──────────────

type LocalObjectStore struct {
	baseDir    string
	signSecret string
}

func NewLocalObjectStore(baseDir, signSecret string) *LocalObjectStore {
	if err := os.MkdirAll(filepath.Join(baseDir, "uploads"), 0755); err != nil {
		slog.Error("create uploads dir", "error", err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "objects"), 0755); err != nil {
		slog.Error("create objects dir", "error", err)
	}
	return &LocalObjectStore{baseDir: baseDir, signSecret: signSecret}
}

func (s *LocalObjectStore) InitiateMultipartUpload(_ context.Context, key string, _ string) (string, error) {
	uploadID := fmt.Sprintf("upload_%d", time.Now().UnixNano())
	dir := filepath.Join(s.baseDir, "uploads", uploadID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("init upload: %w", err)
	}
	meta := map[string]string{"key": key}
	metaJSON, _ := json.Marshal(meta)
	os.WriteFile(filepath.Join(dir, "meta.json"), metaJSON, 0644)
	return uploadID, nil
}

func (s *LocalObjectStore) PresignUploadPart(_ context.Context, key, uploadID string, partNumber int, expires time.Duration) (string, error) {
	exp := time.Now().Add(expires).Unix()
	sig := s.sign(fmt.Sprintf("%s|%s|%d|%d", key, uploadID, partNumber, exp))
	return fmt.Sprintf("/media/upload-part/%s/%d?exp=%d&sig=%s", uploadID, partNumber, exp, sig), nil
}

func (s *LocalObjectStore) CompleteMultipartUpload(_ context.Context, key, uploadID string, parts []Part) error {
	dir := filepath.Join(s.baseDir, "uploads", uploadID)
	destDir := filepath.Join(s.baseDir, "objects", filepath.Dir(key))
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}
	dest := filepath.Join(s.baseDir, "objects", key)

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	for _, part := range parts {
		partFile := filepath.Join(dir, fmt.Sprintf("part_%d", part.PartNumber))
		f, err := os.Open(partFile)
		if err != nil {
			return fmt.Errorf("open part %d: %w", part.PartNumber, err)
		}
		if _, err := io.Copy(out, f); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}
	_ = s.CleanupUpload(context.Background(), key, uploadID)
	return nil
}

func (s *LocalObjectStore) ListCompletedParts(_ context.Context, _, uploadID string) ([]Part, error) {
	dir := filepath.Join(s.baseDir, "uploads", uploadID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var parts []Part
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "part_") {
			var partNum int
			fmt.Sscanf(e.Name(), "part_%d", &partNum)
			parts = append(parts, Part{PartNumber: partNum, ETag: e.Name()})
		}
	}
	return parts, nil
}

func (s *LocalObjectStore) GetObject(_ context.Context, key string) ([]byte, error) {
	path := filepath.Join(s.baseDir, "objects", key)
	return os.ReadFile(path)
}

func (s *LocalObjectStore) PutObject(_ context.Context, key string, data []byte, _ string) error {
	destDir := filepath.Join(s.baseDir, "objects", filepath.Dir(key))
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.baseDir, "objects", key), data, 0644)
}

func (s *LocalObjectStore) PresignDownload(_ context.Context, key string, expires time.Duration) (string, error) {
	exp := time.Now().Add(expires).Unix()
	sig := s.sign(fmt.Sprintf("%s|%d", key, exp))
	return fmt.Sprintf("/media/download/%s?exp=%d&sig=%s", urlEncode(key), exp, sig), nil
}

func urlEncode(s string) string {
	// Simple URL-safe encoding
	return strings.ReplaceAll(strings.ReplaceAll(s, "/", "_"), "+", "-")
}

func (s *LocalObjectStore) CleanupUpload(_ context.Context, _, uploadID string) error {
	dir := filepath.Join(s.baseDir, "uploads", uploadID)
	return os.RemoveAll(dir)
}

func (s *LocalObjectStore) sign(payload string) string {
	mac := hmac.New(sha256.New, []byte(s.signSecret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// ── Service ───────────────────────────────────────

type Service struct {
	db    *sql.DB
	store ObjectStore
}

func NewService(db *sql.DB, store ObjectStore) *Service {
	return &Service{db: db, store: store}
}

const (
	maxFileSize    = 50 * 1024 * 1024 // 50MB
	chunkSize      = 5 * 1024 * 1024  // 5MB
	uploadExpiry   = 15 * time.Minute
	downloadExpiry = 1 * time.Hour

	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusComplete   = "complete"
	StatusFailed     = "failed"
	StatusOrphan     = "orphan"
)

var validMIMETypes = map[string]bool{
	"image/jpeg": true,
}

// ── Request / Response types ──────────────────────

type UploadInitiateReq struct {
	MimeType       string `json:"mime_type"`
	SizeBytes      int64  `json:"size_bytes"`
	FileName       string `json:"file_name"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	ConversationID string `json:"conversation_id"`
}

type UploadInitiateResp struct {
	UploadID      string   `json:"upload_id"`
	ObjectKey     string   `json:"object_key"`
	ChunkSize     int      `json:"chunk_size"`
	PresignedURLs []string `json:"presigned_urls"`
	ExpiresAtMs   int64    `json:"expires_at_ms"`
}

type UploadStatusResp struct {
	UploadID       string `json:"upload_id"`
	ObjectKey      string `json:"object_key"`
	Status         string `json:"status"`
	CompletedParts []Part `json:"completed_parts"`
	ExpiresAtMs    int64  `json:"expires_at_ms"`
}

type DownloadAuthReq struct {
	ObjectKey      string `json:"object_key"`
	ConversationID string `json:"conversation_id"`
}

type DownloadAuthResp struct {
	DownloadURL   string `json:"download_url"`
	ExpiresInSec  int64  `json:"expires_in_sec"`
	ContentLength int64  `json:"content_length"`
	ContentType   string `json:"content_type"`
}

// ── Upload flow ───────────────────────────────────

func (s *Service) InitiateUpload(ctx context.Context, userID int64, req UploadInitiateReq) (*UploadInitiateResp, error) {
	if req.SizeBytes > maxFileSize {
		return nil, fmt.Errorf("file too large: %d bytes (max %d)", req.SizeBytes, maxFileSize)
	}
	if !validMIMETypes[req.MimeType] {
		return nil, fmt.Errorf("unsupported mime type: %s", req.MimeType)
	}

	objectKey := fmt.Sprintf("media/u_%d/%s/img_%d_%s",
		userID, time.Now().Format("2006/01/02"), time.Now().UnixNano(), req.FileName)

	uploadID, err := s.store.InitiateMultipartUpload(ctx, objectKey, req.MimeType)
	if err != nil {
		return nil, err
	}

	numParts := int(req.SizeBytes+chunkSize-1) / chunkSize
	pURLs := make([]string, 0, numParts)
	for i := 0; i < numParts; i++ {
		url, err := s.store.PresignUploadPart(ctx, objectKey, uploadID, i+1, uploadExpiry)
		if err != nil {
			return nil, err
		}
		pURLs = append(pURLs, url)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO attachments (object_key, mime_type, size_bytes, width, height, upload_status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`,
		objectKey, req.MimeType, req.SizeBytes, req.Width, req.Height, StatusPending,
	)
	if err != nil {
		return nil, fmt.Errorf("record attachment: %w", err)
	}

	return &UploadInitiateResp{
		UploadID:      uploadID,
		ObjectKey:     objectKey,
		ChunkSize:     chunkSize,
		PresignedURLs: pURLs,
		ExpiresAtMs:   time.Now().Add(uploadExpiry).UnixMilli(),
	}, nil
}

func (s *Service) UploadStatus(ctx context.Context, uploadID string) (*UploadStatusResp, error) {
	var objectKey string
	err := s.db.QueryRowContext(ctx,
		"SELECT object_key FROM attachments WHERE object_key IS NOT NULL ORDER BY created_at DESC LIMIT 1",
	).Scan(&objectKey)
	if err != nil {
		return nil, fmt.Errorf("find attachment: %w", err)
	}

	parts, err := s.store.ListCompletedParts(ctx, objectKey, uploadID)
	if err != nil {
		return nil, err
	}

	return &UploadStatusResp{
		UploadID:       uploadID,
		ObjectKey:      objectKey,
		Status:         "in_progress",
		CompletedParts: parts,
		ExpiresAtMs:    time.Now().Add(uploadExpiry).UnixMilli(),
	}, nil
}

func (s *Service) CompleteUpload(ctx context.Context, objectKey, uploadID string, parts []Part) error {
	err := s.store.CompleteMultipartUpload(ctx, objectKey, uploadID, parts)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE attachments SET upload_status=$1, updated_at=NOW() WHERE object_key=$2`,
		StatusProcessing, objectKey,
	)
	return err
}

// GenerateThumbnail reads from store, produces a 320px-wide JPEG thumbnail,
// writes it back, and updates the attachment row.
func (s *Service) GenerateThumbnail(ctx context.Context, objectKey string) error {
	s.db.ExecContext(ctx,
		`UPDATE attachments SET upload_status=$1, updated_at=NOW() WHERE object_key=$2`,
		StatusProcessing, objectKey)

	raw, err := s.store.GetObject(ctx, objectKey)
	if err != nil {
		s.markFailed(ctx, objectKey)
		return fmt.Errorf("get object: %w", err)
	}

	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		s.markFailed(ctx, objectKey)
		return fmt.Errorf("decode image: %w", err)
	}

	thumbKey := strings.Replace(objectKey, "_original.", "_thumb_320.", 1)
	if thumbKey == objectKey {
		thumbKey = strings.Replace(objectKey, "img_", "thumb_", 1)
	}
	bounds := img.Bounds()
	newWidth := 320
	newHeight := int(float64(bounds.Dy()) * float64(newWidth) / float64(bounds.Dx()))
	if newHeight < 1 {
		newHeight = 1
	}
	resized := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw.ApproxBiLinear.Scale(resized, resized.Bounds(), img, bounds, draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: 80}); err != nil {
		s.markFailed(ctx, objectKey)
		return fmt.Errorf("encode thumbnail: %w", err)
	}

	if err := s.store.PutObject(ctx, thumbKey, buf.Bytes(), "image/jpeg"); err != nil {
		return fmt.Errorf("put thumbnail: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE attachments SET thumbnail_key=$1, upload_status=$2, updated_at=NOW() WHERE object_key=$3`,
		thumbKey, StatusComplete, objectKey,
	)
	return err
}

func (s *Service) markFailed(ctx context.Context, objectKey string) {
	s.db.ExecContext(ctx,
		`UPDATE attachments SET upload_status=$1, updated_at=NOW() WHERE object_key=$2`,
		StatusFailed, objectKey)
}

// AuthorizeDownload verifies conversation membership and returns a presigned URL.
func (s *Service) AuthorizeDownload(ctx context.Context, userID int64, req DownloadAuthReq) (*DownloadAuthResp, error) {
	var ok bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id=$1 AND user_id=$2)`,
		req.ConversationID, userID,
	).Scan(&ok)
	if err != nil || !ok {
		return nil, fmt.Errorf("not authorized to download from this conversation")
	}

	var mimeType string
	var sizeBytes int64
	err = s.db.QueryRowContext(ctx,
		`SELECT mime_type, size_bytes FROM attachments WHERE object_key=$1 AND upload_status=$2`,
		req.ObjectKey, StatusComplete,
	).Scan(&mimeType, &sizeBytes)
	if err != nil {
		return nil, fmt.Errorf("attachment not found or not ready")
	}

	url, err := s.store.PresignDownload(ctx, req.ObjectKey, downloadExpiry)
	if err != nil {
		return nil, err
	}

	return &DownloadAuthResp{
		DownloadURL:   url,
		ExpiresInSec:  int64(downloadExpiry.Seconds()),
		ContentLength: sizeBytes,
		ContentType:   mimeType,
	}, nil
}

// CleanupOrphans marks attachments that have been pending for >24 hours as orphaned.
func (s *Service) CleanupOrphans(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE attachments SET upload_status=$1, updated_at=NOW()
		 WHERE upload_status=$2 AND created_at < NOW() - INTERVAL '24 hours'`,
		StatusOrphan, StatusPending,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ── Sign helpers ──────────────────────────────────

func (s *Service) SignDownload(key string, expiresAt int64) string {
	return s.sign(fmt.Sprintf("%s|%d", key, expiresAt))
}

func (s *Service) sign(payload string) string {
	return s.store.(*LocalObjectStore).sign(payload)
}

func (s *Service) Store() ObjectStore {
	return s.store
}
