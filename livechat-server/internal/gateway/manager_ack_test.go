package gateway

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/auth"
	livechat "github.com/tangzzz-fan/LiveChat/livechat-server/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

func TestGatewayForwardsReadAckToMessageService(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	defer rdb.Close()
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("redis not available: %v", err)
	}

	ackCh := make(chan *livechat.ProcessAckRequest, 1)
	lis := bufconn.Listen(1024 * 1024)
	grpcSrv := grpc.NewServer()
	livechat.RegisterMessageAckServiceServer(grpcSrv, &recordingAckServer{ackCh: ackCh})
	go func() {
		_ = grpcSrv.Serve(lis)
	}()
	defer grpcSrv.Stop()

	grpcConn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithInsecure(),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	defer grpcConn.Close()

	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	manager := NewManager(authSvc, rdb, "node-test", 30*time.Second, 90*time.Second)
	manager.SetAckForwarder(NewGRPCAckForwarderClient(livechat.NewMessageAckServiceClient(grpcConn)))

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", manager.HandleUpgrade)
	server := httptest.NewServer(mux)
	defer server.Close()

	accessToken, err := authSvc.SignAccessToken(101, "ios-a", 1)
	if err != nil {
		t.Fatalf("SignAccessToken: %v", err)
	}

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer wsConn.Close()

	hsFrame, err := NewFrame(OpHandshakeReq, &livechat.HandshakeRequest{
		AccessToken: accessToken,
		DeviceId:    "ios-a",
		Platform:    "ios",
	})
	if err != nil {
		t.Fatalf("NewFrame handshake: %v", err)
	}
	raw, err := MarshalFrame(hsFrame)
	if err != nil {
		t.Fatalf("MarshalFrame handshake: %v", err)
	}
	if err := wsConn.WriteMessage(websocket.BinaryMessage, raw); err != nil {
		t.Fatalf("WriteMessage handshake: %v", err)
	}
	if _, _, err := wsConn.ReadMessage(); err != nil {
		t.Fatalf("ReadMessage handshake resp: %v", err)
	}

	ackFrame, err := NewFrame(OpAck, &livechat.MessageAck{
		AckType:        "read",
		ConversationId: "conv-ack",
		LastReadSeq:    7,
		AckedAtMs:      uint64(time.Now().UnixMilli()),
	})
	if err != nil {
		t.Fatalf("NewFrame ack: %v", err)
	}
	raw, err = MarshalFrame(ackFrame)
	if err != nil {
		t.Fatalf("MarshalFrame ack: %v", err)
	}
	if err := wsConn.WriteMessage(websocket.BinaryMessage, raw); err != nil {
		t.Fatalf("WriteMessage ack: %v", err)
	}

	select {
	case got := <-ackCh:
		if got.GetAckType() != "read" {
			t.Fatalf("unexpected ack_type: %v", got.GetAckType())
		}
		if got.GetConversationId() != "conv-ack" {
			t.Fatalf("unexpected conversation_id: %v", got.GetConversationId())
		}
		if got.GetUserId() != 101 {
			t.Fatalf("unexpected user_id: %v", got.GetUserId())
		}
		if got.GetDeviceId() != "ios-a" {
			t.Fatalf("unexpected device_id: %v", got.GetDeviceId())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for forwarded ack")
	}
}

type recordingAckServer struct {
	livechat.UnimplementedMessageAckServiceServer
	ackCh chan *livechat.ProcessAckRequest
}

func (s *recordingAckServer) ProcessAck(_ context.Context, req *livechat.ProcessAckRequest) (*livechat.ProcessAckResponse, error) {
	s.ackCh <- req
	return &livechat.ProcessAckResponse{Accepted: true}, nil
}
