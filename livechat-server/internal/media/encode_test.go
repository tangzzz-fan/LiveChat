package media

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestObjectKeyURLEncodeRoundTripPreservesUnderscores(t *testing.T) {
	key := "media/u_1201/2026/07/27/img_1785163851196381000_photo.jpg"
	encoded := urlEncode(key)
	decoded, err := urlDecode(encoded)
	if err != nil {
		t.Fatalf("urlDecode: %v", err)
	}
	if decoded != key {
		t.Fatalf("lossy object_key encoding:\n  want %q\n  got  %q\n  via  %q", key, decoded, encoded)
	}
}

func TestPresignDownloadServeRoundTripWithUnderscoreKey(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalObjectStore(dir, "test-download-sign-secret")
	key := "media/u_1201/2026/07/27/img_1785163851196381000_photo.jpg"
	payload := []byte("jpeg-bytes-not-real-but-ok")
	if err := store.PutObject(t.Context(), key, payload, "image/jpeg"); err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	rawURL, err := store.PresignDownload(t.Context(), key, time.Hour)
	if err != nil {
		t.Fatalf("PresignDownload: %v", err)
	}
	if strings.Contains(rawURL, "/") && strings.Count(strings.TrimPrefix(rawURL, "/media/download/"), "/") > 0 {
		// path segment after /media/download/ must stay a single segment (no raw slashes from key)
		pathOnly := strings.SplitN(rawURL, "?", 2)[0]
		rest := strings.TrimPrefix(pathOnly, "/media/download/")
		if strings.Contains(rest, "/") {
			t.Fatalf("encoded key leaked path separators: %s", rawURL)
		}
	}

	req := httptest.NewRequest(http.MethodGet, rawURL, nil)
	rec := httptest.NewRecorder()
	svc := &Service{store: store, db: nil}
	if err := svc.ServeDownload(rec, req); err != nil {
		t.Fatalf("ServeDownload failed for underscore-rich key (receiver symptom): %v\nurl=%s", err, rawURL)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body, _ := io.ReadAll(rec.Body)
	if string(body) != string(payload) {
		t.Fatalf("body mismatch: got %q", body)
	}

	// Sanity: object file is where LocalObjectStore expects it.
	if _, err := store.GetObject(t.Context(), key); err != nil {
		t.Fatalf("GetObject after put: %v (path=%s)", err, filepath.Join(dir, "objects", key))
	}
}

func TestUrlDecodeRejectsGarbage(t *testing.T) {
	if _, err := urlDecode("%%%not-valid%%%"); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestLegacyLossyEncodeWouldBreakSignature(t *testing.T) {
	// Documents the pre-fix bug: `/`↔`_` is not injective when keys contain `_`.
	key := "media/u_1201/2026/07/27/img_1785163851196381000_photo.jpg"
	legacyEnc := strings.ReplaceAll(strings.ReplaceAll(key, "/", "_"), "+", "-")
	legacyDec := strings.ReplaceAll(strings.ReplaceAll(legacyEnc, "_", "/"), "-", "+")
	if legacyDec == key {
		t.Fatal("fixture no longer demonstrates lossy legacy encoding")
	}
	store := NewLocalObjectStore(t.TempDir(), "secret")
	exp := time.Now().Add(time.Hour).Unix()
	sig := store.sign(fmt.Sprintf("%s|%d", key, exp))
	if store.VerifySignature(legacyDec, fmt.Sprintf("%d", exp), sig) {
		t.Fatal("legacy-decoded key should not verify against signature of original key")
	}
}
