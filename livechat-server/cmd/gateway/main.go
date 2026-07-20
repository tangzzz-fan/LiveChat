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

	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/auth"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/gateway"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/infra"
	syncsvc "github.com/tangzzz-fan/LiveChat/livechat-server/internal/sync"
	livechat "github.com/tangzzz-fan/LiveChat/livechat-server/proto"
	"google.golang.org/grpc"
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
		30*time.Second, // heartbeat interval
		90*time.Second, // read timeout
	)
	gwMgr.SetSyncSequenceProvider(syncsvc.NewService(db))
	ackForwarder, err := gateway.NewGRPCAckForwarder("localhost:9090")
	if err != nil {
		slog.Error("failed to create ack forwarder", "error", err)
		os.Exit(1)
	}
	defer ackForwarder.Close()
	gwMgr.SetAckForwarder(ackForwarder)

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
	grpcLis, err := net.Listen("tcp", ":9091")
	if err != nil {
		slog.Error("gateway gRPC listen failed", "error", err)
		os.Exit(1)
	}
	grpcSrv := grpc.NewServer()
	livechat.RegisterGatewayDeliveryServiceServer(grpcSrv, gateway.NewDeliveryService(gwMgr))

	go func() {
		slog.Info("gateway starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("gateway error", "error", err)
			os.Exit(1)
		}
	}()
	go func() {
		slog.Info("gateway gRPC starting", "addr", grpcLis.Addr().String())
		if err := grpcSrv.Serve(grpcLis); err != nil {
			slog.Error("gateway gRPC error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("gateway shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
	grpcSrv.GracefulStop()
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
