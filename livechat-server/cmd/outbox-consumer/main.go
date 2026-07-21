package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/conversations"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/domain"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/fanout"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/infra"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/metrics"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/outbox"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/receipts"
	syncsvc "github.com/tangzzz-fan/LiveChat/livechat-server/internal/sync"
	livechat "github.com/tangzzz-fan/LiveChat/livechat-server/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
	syncSvc := syncsvc.NewService(db)
	receiptSvc := receipts.NewService(db, syncSvc, conversations.NewService(db))
	deliverer := newGRPCDeliverer(rdb)
	defer deliverer.Close()
	fanoutSvc := fanout.NewService(db, rdb, deliverer, syncSvc)

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
		err := fanoutSvc.Fanout(ctx, domainEvent)
		if errors.Is(err, fanout.ErrGroupBusy) {
			// Hot group: do not retry, event was dropped intentionally
			slog.Warn("hot group event dropped", "aggregate_id", event.AggregateID)
			return nil
		}
		return err
	})

	consumer.RegisterHandler("delivery_acked", func(ctx context.Context, event outbox.Event) error {
		slog.Info("delivery acked", "aggregate_id", event.AggregateID)
		return nil
	})

	consumer.RegisterHandler("read_receipt", func(ctx context.Context, event outbox.Event) error {
		var payload receipts.ReadReceiptPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("unmarshal read receipt payload: %w", err)
		}
		return receiptSvc.ConsumeReadReceipt(ctx, payload)
	})

	metricsSrv := &http.Server{
		Addr:    ":8082",
		Handler: metrics.Handler(func() map[string]int64 { return consumer.Metrics(context.Background()) }),
	}
	go func() {
		slog.Info("outbox metrics server starting", "addr", metricsSrv.Addr)
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("outbox metrics server error", "error", err)
			os.Exit(1)
		}
	}()

	slog.Info("outbox-consumer starting")
	if err := consumer.Run(ctx); err != nil && err != context.Canceled {
		slog.Error("consumer error", "error", err)
		os.Exit(1)
	}
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelShutdown()
	_ = metricsSrv.Shutdown(shutdownCtx)
	slog.Info("outbox-consumer stopped")
}

type grpcDeliverer struct {
	rdb   *redis.Client
	mu    sync.Mutex
	conns map[string]*grpc.ClientConn
}

func newGRPCDeliverer(rdb *redis.Client) *grpcDeliverer {
	return &grpcDeliverer{
		rdb:   rdb,
		conns: make(map[string]*grpc.ClientConn),
	}
}

func (d *grpcDeliverer) DeliverMessage(ctx context.Context, userID int64, deviceID string, payload *fanout.DeliveryPayload) error {
	route, err := d.rdb.Get(ctx, redisUserKey(userID, deviceID)).Result()
	if err != nil {
		return err
	}
	nodeID, _, ok := splitRoute(route)
	if !ok {
		return fmt.Errorf("invalid gateway route value: %q", route)
	}
	client, err := d.clientForNode(nodeID)
	if err != nil {
		return err
	}
	_, err = client.DeliverMessage(ctx, &livechat.DeliverMessageRequest{
		UserId:   userID,
		DeviceId: deviceID,
		Message: &livechat.WsMessageDelivery{
			ServerMessageId:    payload.ServerMessageID,
			ConversationId:     payload.ConversationID,
			ConversationSeq:    uint64(payload.ConversationSeq),
			SenderUserId:       uint64(payload.SenderUserID),
			SenderDeviceId:     payload.SenderDeviceID,
			MessageType:        payload.MessageType,
			Content:            payload.Content,
			ServerReceivedAtMs: payload.ServerReceivedAtMs,
		},
		TraceId: payload.TraceID,
	})
	return err
}

func redisUserKey(userID int64, deviceID string) string {
	return fmt.Sprintf("gateway:user:%d:%s", userID, deviceID)
}

func splitRoute(route string) (string, string, bool) {
	for i := 0; i < len(route); i++ {
		if route[i] == ':' {
			return route[:i], route[i+1:], route[:i] != ""
		}
	}
	return "", "", false
}

func (d *grpcDeliverer) clientForNode(nodeID string) (livechat.GatewayDeliveryServiceClient, error) {
	addr, err := gatewayGRPCAddr(nodeID)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	conn, ok := d.conns[addr]
	if !ok {
		conn, err = grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, fmt.Errorf("dial gateway grpc %s: %w", addr, err)
		}
		d.conns[addr] = conn
	}
	return livechat.NewGatewayDeliveryServiceClient(conn), nil
}

func (d *grpcDeliverer) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	var firstErr error
	for addr, conn := range d.conns {
		if err := conn.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close grpc conn %s: %w", addr, err)
		}
		delete(d.conns, addr)
	}
	return firstErr
}

func gatewayGRPCAddr(nodeID string) (string, error) {
	switch nodeID {
	case "node-1":
		return "localhost:9091", nil
	default:
		return "", fmt.Errorf("unknown gateway node id: %s", nodeID)
	}
}

// summaryWriter maintains conversation_summaries.
// Full implementation in ticket 0008.
type summaryWriter struct{ db *sql.DB }
