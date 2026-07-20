package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/infra"
	syncsvc "github.com/tangzzz-fan/LiveChat/livechat-server/internal/sync"
)

func main() {
	db, err := infra.NewDB(infra.DBConfig{
		Host:            "localhost",
		Port:            5432,
		User:            "livechat",
		Password:        "livechat",
		Name:            "livechat",
		SSLMode:         "disable",
		MaxOpenConns:    2,
		MaxIdleConns:    1,
		ConnMaxLifetime: 5 * time.Minute,
	})
	if err != nil {
		slog.Error("failed to connect database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	retentionDays := 30
	if raw := os.Getenv("SYNC_EVENT_RETENTION_DAYS"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			retentionDays = parsed
		}
	}

	cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	deleted, err := syncsvc.NewService(db).DeleteEventsOlderThan(context.Background(), cutoff)
	if err != nil {
		slog.Error("sync retention cleanup failed", "error", err, "retention_days", retentionDays)
		os.Exit(1)
	}

	slog.Info("sync retention cleanup completed",
		"retention_days", retentionDays,
		"deleted_rows", deleted,
		"cutoff", cutoff.UTC().Format(time.RFC3339),
	)
}
