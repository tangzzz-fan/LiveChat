package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/infra"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/outbox"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

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

	consumer := outbox.NewConsumer(db, outbox.Config{
		PollInterval:     100 * time.Millisecond,
		IdlePollInterval: 500 * time.Millisecond,
		BatchSize:        100,
		MaxRetries:       10,
		WorkerCount:      4,
		LeaseTimeout:     60 * time.Second,
	})

	// Register stub handlers that just log and mark done.
	// These will be replaced by real handlers in tickets 0006/0008/0009.
	consumer.RegisterHandler("message_created", stubHandler("message_created"))
	consumer.RegisterHandler("delivery_acked", stubHandler("delivery_acked"))
	consumer.RegisterHandler("read_receipt", stubHandler("read_receipt"))

	slog.Info("outbox-consumer starting")
	if err := consumer.Run(ctx); err != nil && err != context.Canceled {
		slog.Error("consumer error", "error", err)
		os.Exit(1)
	}
	slog.Info("outbox-consumer stopped")
}

func stubHandler(name string) outbox.Handler {
	return func(ctx context.Context, event outbox.Event) error {
		slog.Info("stub handler", "handler", name, "event_id", event.ID, "aggregate_id", event.AggregateID)
		return nil
	}
}
