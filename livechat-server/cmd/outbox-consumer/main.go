package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/domain"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/fanout"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/infra"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/outbox"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/sync"
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

	// Services
	syncSvc := sync.NewService(db)
	fanoutSvc := fanout.NewService(db, rdb, &logDeliverer{}, syncSvc)

	consumer := outbox.NewConsumer(db, outbox.Config{
		PollInterval:     100 * time.Millisecond,
		IdlePollInterval: 500 * time.Millisecond,
		BatchSize:        100,
		MaxRetries:       10,
		WorkerCount:      4,
		LeaseTimeout:     60 * time.Second,
	})

	// Register real handlers
	consumer.RegisterHandler("message_created", func(ctx context.Context, event outbox.Event) error {
		var domainEvent domain.OutboxEvent
		domainEvent.ID = event.ID
		domainEvent.AggregateType = event.AggregateType
		domainEvent.AggregateID = event.AggregateID
		domainEvent.EventType = event.EventType
		domainEvent.Payload = string(event.Payload)
		domainEvent.Status = event.Status
		domainEvent.RetryCount = event.RetryCount
		domainEvent.CreatedAt = event.CreatedAt
		return fanoutSvc.Fanout(ctx, domainEvent)
	})

	consumer.RegisterHandler("delivery_acked", func(ctx context.Context, event outbox.Event) error {
		slog.Info("delivery acked", "aggregate_id", event.AggregateID)
		return nil
	})

	consumer.RegisterHandler("read_receipt", func(ctx context.Context, event outbox.Event) error {
		slog.Info("read receipt", "aggregate_id", event.AggregateID)
		return nil
	})

	slog.Info("outbox-consumer starting")
	if err := consumer.Run(ctx); err != nil && err != context.Canceled {
		slog.Error("consumer error", "error", err)
		os.Exit(1)
	}
	slog.Info("outbox-consumer stopped")
}

// logDeliverer logs delivery instead of gRPC to Gateway (for now).
// In production, this would call Gateway via gRPC.
type logDeliverer struct{}

func (d *logDeliverer) DeliverMessage(ctx context.Context, userID int64, deviceID string, payload *fanout.DeliveryPayload) error {
	slog.Info("message delivered",
		"user_id", userID,
		"device_id", deviceID,
		"server_message_id", payload.ServerMessageID,
		"conversation_id", payload.ConversationID,
		"conversation_seq", payload.ConversationSeq,
	)
	return nil
}

// summaryWriter maintains conversation_summaries.
// Full implementation in ticket 0008.
type summaryWriter struct{ db *sql.DB }
