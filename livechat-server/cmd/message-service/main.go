package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/api"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/auth"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/conversations"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/infra"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/media"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/receipts"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/sync"
	livechat "github.com/tangzzz-fan/LiveChat/livechat-server/proto"
	"google.golang.org/grpc"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Database
	db, err := infra.NewDB(infra.DBConfig{
		Host:            "localhost",
		Port:            5432,
		User:            "livechat",
		Password:        "livechat",
		Name:            "livechat",
		SSLMode:         "disable",
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
	})
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("connected to database")

	// Redis
	rdb, err := infra.NewRedis(infra.RedisConfig{
		Host:     "localhost",
		Port:     6379,
		Password: "",
		DB:       0,
	})
	if err != nil {
		slog.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer rdb.Close()
	slog.Info("connected to redis")

	// Auth
	authSvc := auth.NewService(
		"livechat-dev-secret-change-in-production",
		1*time.Hour,
		30*24*time.Hour,
	)

	// Media store (P0: local filesystem)
	mediaStore := media.NewLocalObjectStore("data/storage", "livechat-dev-media-sign-secret")
	mediaSvc := media.NewService(db, mediaStore)

	// Thumbnail worker: process thumbnail jobs from a buffered channel
	thumbnailCh := make(chan media.ThumbnailJob, 64)
	mediaSvc.SetThumbnailChannel(thumbnailCh)
	go func() {
		for job := range thumbnailCh {
			if err := mediaSvc.GenerateThumbnail(context.Background(), job.ObjectKey); err != nil {
				slog.Error("thumbnail generation failed", "object_key", job.ObjectKey, "error", err)
			} else {
				slog.Info("thumbnail generated", "object_key", job.ObjectKey)
			}
		}
	}()

	// Orphan cleanup: every 10 minutes, mark stale attachments as orphan
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				rows, dirs, err := mediaSvc.CleanupOrphans(context.Background())
				if err != nil {
					slog.Error("orphan cleanup failed", "error", err)
				} else {
					slog.Info("orphan cleanup done", "rows_orphaned", rows, "dirs_cleaned", dirs)
				}
			}
		}
	}()

	// Router
	mux := api.NewRouter(db, rdb, authSvc, mediaSvc)
	syncSvc := sync.NewService(db)
	receiptSvc := receipts.NewService(db, syncSvc, conversations.NewService(db))

	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	grpcLis, err := net.Listen("tcp", ":9090")
	if err != nil {
		slog.Error("message-service gRPC listen failed", "error", err)
		os.Exit(1)
	}
	grpcSrv := grpc.NewServer()
	livechat.RegisterMessageAckServiceServer(grpcSrv, receipts.NewGRPCServer(receiptSvc))

	go func() {
		slog.Info("message-service starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()
	go func() {
		slog.Info("message-service gRPC starting", "addr", grpcLis.Addr().String())
		if err := grpcSrv.Serve(grpcLis); err != nil {
			slog.Error("message-service gRPC error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
	grpcSrv.GracefulStop()
	slog.Info("stopped")
}
