package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/auth"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/gateway"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/infra"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Database (for auth lookup)
	db, err := infra.NewDB(infra.DBConfig{
		Host:            "localhost",
		Port:            5432,
		User:            "livechat",
		Password:        "livechat",
		Name:            "livechat",
		SSLMode:         "disable",
		MaxOpenConns:    10,
		MaxIdleConns:    2,
		ConnMaxLifetime: 5 * time.Minute,
	})
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

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

	authSvc := auth.NewService(
		"livechat-dev-secret-change-in-production",
		1*time.Hour,
		30*24*time.Hour,
	)

	gwMgr := gateway.NewManager(
		authSvc, rdb, "node-1",
		30*time.Second,  // heartbeat interval
		90*time.Second,  // read timeout
	)

	// Start watchdog
	go gwMgr.StartWatchdog(ctx)

	// HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", gwMgr.HandleUpgrade)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","active_sessions":` +
			itoa(gwMgr.ActiveSessions()) + `}`))
	})

	srv := &http.Server{
		Addr:    ":8081",
		Handler: mux,
	}

	go func() {
		slog.Info("gateway starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("gateway error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("gateway shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
	slog.Info("gateway stopped")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := make([]byte, 0, 20)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
