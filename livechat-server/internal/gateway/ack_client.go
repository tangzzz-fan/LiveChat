package gateway

import (
	"context"
	"fmt"

	livechat "github.com/tangzzz-fan/LiveChat/livechat-server/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type AckForwarder interface {
	ForwardAck(ctx context.Context, userID int64, deviceID string, ack *livechat.MessageAck, traceID string) error
}

type GRPCAckForwarder struct {
	conn   *grpc.ClientConn
	client livechat.MessageAckServiceClient
}

func NewGRPCAckForwarder(target string) (*GRPCAckForwarder, error) {
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial message ack service: %w", err)
	}
	return &GRPCAckForwarder{
		conn:   conn,
		client: livechat.NewMessageAckServiceClient(conn),
	}, nil
}

func NewGRPCAckForwarderClient(client livechat.MessageAckServiceClient) *GRPCAckForwarder {
	return &GRPCAckForwarder{client: client}
}

func (f *GRPCAckForwarder) Close() error {
	if f.conn == nil {
		return nil
	}
	return f.conn.Close()
}

func (f *GRPCAckForwarder) ForwardAck(ctx context.Context, userID int64, deviceID string, ack *livechat.MessageAck, traceID string) error {
	if f.client == nil {
		return fmt.Errorf("message ack gRPC client is not configured")
	}
	_, err := f.client.ProcessAck(ctx, &livechat.ProcessAckRequest{
		UserId:         userID,
		DeviceId:       deviceID,
		AckType:        ack.GetAckType(),
		EventSeq:       ack.GetEventSeq(),
		ConversationId: ack.GetConversationId(),
		LastReadSeq:    ack.GetLastReadSeq(),
		AckedAtMs:      ack.GetAckedAtMs(),
		TraceId:        traceID,
	})
	if err != nil {
		return fmt.Errorf("process ack over grpc: %w", err)
	}
	return nil
}
