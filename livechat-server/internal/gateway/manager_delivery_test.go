package gateway

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/auth"
	livechat "github.com/tangzzz-fan/LiveChat/livechat-server/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

func TestGatewayDeliversPublishedMessageToConnectedDevice(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	defer rdb.Close()
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("redis not available: %v", err)
	}

	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	manager := NewManager(authSvc, rdb, "node-test", 30*time.Second, 90*time.Second)
	manager.SetSyncSequenceProvider(staticSyncSequenceProvider(42))
	lis := bufconn.Listen(1024 * 1024)
	grpcSrv := grpc.NewServer()
	livechat.RegisterGatewayDeliveryServiceServer(grpcSrv, NewDeliveryService(manager))
	go func() {
		_ = grpcSrv.Serve(lis)
	}()
	defer grpcSrv.Stop()

	connGRPC, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithInsecure(),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	defer connGRPC.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", manager.HandleUpgrade)
	server := httptest.NewServer(mux)
	defer server.Close()

	accessToken, err := authSvc.SignAccessToken(101, "ios-a")
	if err != nil {
		t.Fatalf("SignAccessToken: %v", err)
	}

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	req := &livechat.HandshakeRequest{
		AccessToken: accessToken,
		DeviceId:    "ios-a",
		Platform:    "ios",
	}
	frame, err := NewFrame(OpHandshakeReq, req)
	if err != nil {
		t.Fatalf("NewFrame handshake: %v", err)
	}
	raw, err := MarshalFrame(frame)
	if err != nil {
		t.Fatalf("MarshalFrame handshake: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, raw); err != nil {
		t.Fatalf("WriteMessage handshake: %v", err)
	}

	_, raw, err = conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage handshake resp: %v", err)
	}
	respFrame, err := UnmarshalFrame(raw)
	if err != nil {
		t.Fatalf("UnmarshalFrame handshake resp: %v", err)
	}
	if respFrame.Opcode != OpHandshakeResp {
		t.Fatalf("unexpected handshake opcode: got %d", respFrame.Opcode)
	}
	hsResp := &livechat.HandshakeResponse{}
	if err := proto.Unmarshal(respFrame.Payload, hsResp); err != nil {
		t.Fatalf("proto.Unmarshal handshake payload: %v", err)
	}
	if hsResp.GetLatestEventSeq() != 42 {
		t.Fatalf("unexpected latest_event_seq: got %d", hsResp.GetLatestEventSeq())
	}

	payload := &livechat.WsMessageDelivery{
		ServerMessageId:    "msg-1",
		ConversationId:     "conv-1",
		ConversationSeq:    1,
		SenderUserId:       202,
		SenderDeviceId:     "android-b",
		MessageType:        "text",
		Content:            "{\"text\":\"hello\"}",
		ServerReceivedAtMs: time.Now().UnixMilli(),
	}
	client := livechat.NewGatewayDeliveryServiceClient(connGRPC)
	if _, err := client.DeliverMessage(context.Background(), &livechat.DeliverMessageRequest{
		UserId:   101,
		DeviceId: "ios-a",
		Message:  payload,
	}); err != nil {
		t.Fatalf("DeliverMessage: %v", err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	_, raw, err = conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage delivery: %v", err)
	}
	deliveryFrame, err := UnmarshalFrame(raw)
	if err != nil {
		t.Fatalf("UnmarshalFrame delivery: %v", err)
	}
	if deliveryFrame.Opcode != OpMessageDelivery {
		t.Fatalf("unexpected delivery opcode: got %d", deliveryFrame.Opcode)
	}

	msg := &livechat.WsMessageDelivery{}
	if err := proto.Unmarshal(deliveryFrame.Payload, msg); err != nil {
		t.Fatalf("proto.Unmarshal delivery payload: %v", err)
	}
	if msg.ServerMessageId != "msg-1" {
		t.Fatalf("unexpected server_message_id: got %s", msg.ServerMessageId)
	}
	if msg.ConversationId != "conv-1" {
		t.Fatalf("unexpected conversation_id: got %s", msg.ConversationId)
	}
}

func TestGatewayReplacesOldSessionWithoutDroppingNewRoute(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	defer rdb.Close()
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("redis not available: %v", err)
	}

	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	manager := NewManager(authSvc, rdb, "node-test", 30*time.Second, 90*time.Second)

	server := newGatewayTestServer(manager)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	accessToken, err := authSvc.SignAccessToken(101, "ios-a")
	if err != nil {
		t.Fatalf("SignAccessToken: %v", err)
	}

	firstConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial first connection: %v", err)
	}
	defer firstConn.Close()
	firstResp := mustHandshakeGatewayConn(t, firstConn, accessToken, "ios-a")

	secondConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial second connection: %v", err)
	}
	defer secondConn.Close()
	secondResp := mustHandshakeGatewayConn(t, secondConn, accessToken, "ios-a")

	if firstResp.GetSessionId() != secondResp.GetSessionId() {
		t.Fatalf("expected same logical session id, got first=%s second=%s", firstResp.GetSessionId(), secondResp.GetSessionId())
	}

	deadline := time.Now().Add(2 * time.Second)
	if err := firstConn.SetReadDeadline(deadline); err != nil {
		t.Fatalf("SetReadDeadline first connection: %v", err)
	}
	_, raw, err := firstConn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage old connection close notice: %v", err)
	}
	oldFrame, err := UnmarshalFrame(raw)
	if err != nil {
		t.Fatalf("UnmarshalFrame old connection close notice: %v", err)
	}
	if oldFrame.Opcode != OpError {
		t.Fatalf("unexpected old connection opcode: %d", oldFrame.Opcode)
	}

	if got := manager.GetSession(101, "ios-a"); got == nil {
		t.Fatalf("new connection is not the active session")
	}
	if manager.ActiveSessions() != 1 {
		t.Fatalf("expected exactly one active session, got %d", manager.ActiveSessions())
	}

	hbFrame, err := NewFrame(OpHeartbeat, &livechat.Heartbeat{})
	if err != nil {
		t.Fatalf("NewFrame heartbeat on replacement: %v", err)
	}
	raw, err = MarshalFrame(hbFrame)
	if err != nil {
		t.Fatalf("MarshalFrame heartbeat on replacement: %v", err)
	}
	if err := secondConn.WriteMessage(websocket.BinaryMessage, raw); err != nil {
		t.Fatalf("WriteMessage heartbeat on replacement: %v", err)
	}
	if err := secondConn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline second connection: %v", err)
	}
	_, raw, err = secondConn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage heartbeat ack on replacement: %v", err)
	}
	secondFrame, err := UnmarshalFrame(raw)
	if err != nil {
		t.Fatalf("UnmarshalFrame heartbeat ack on replacement: %v", err)
	}
	if secondFrame.Opcode != OpHeartbeatAck {
		t.Fatalf("unexpected heartbeat opcode on replacement: %d", secondFrame.Opcode)
	}

	route, err := rdb.Get(context.Background(), redisUserKey(101, "ios-a")).Result()
	if err != nil {
		t.Fatalf("Get redis route: %v", err)
	}
	if route != "node-test:"+secondResp.GetSessionId() {
		t.Fatalf("unexpected route after replacement: %s", route)
	}
}

func TestGatewayHeartbeatRefreshesUserAndNodeRouteTTL(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	defer rdb.Close()
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("redis not available: %v", err)
	}

	authSvc := auth.NewService("test-secret", time.Hour, 24*time.Hour)
	manager := NewManager(authSvc, rdb, "node-test", 30*time.Second, 90*time.Second)

	server := newGatewayTestServer(manager)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	accessToken, err := authSvc.SignAccessToken(101, "ios-a")
	if err != nil {
		t.Fatalf("SignAccessToken: %v", err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	mustHandshakeGatewayConn(t, conn, accessToken, "ios-a")

	ctx := context.Background()
	userKey := redisUserKey(101, "ios-a")
	nodeKey := redisNodeKey("node-test")
	if err := rdb.Expire(ctx, userKey, time.Second).Err(); err != nil {
		t.Fatalf("Expire user key: %v", err)
	}
	if err := rdb.Expire(ctx, nodeKey, time.Second).Err(); err != nil {
		t.Fatalf("Expire node key: %v", err)
	}

	hbFrame, err := NewFrame(OpHeartbeat, &livechat.Heartbeat{})
	if err != nil {
		t.Fatalf("NewFrame heartbeat: %v", err)
	}
	raw, err := MarshalFrame(hbFrame)
	if err != nil {
		t.Fatalf("MarshalFrame heartbeat: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, raw); err != nil {
		t.Fatalf("WriteMessage heartbeat: %v", err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	_, raw, err = conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage heartbeat ack: %v", err)
	}
	ackFrame, err := UnmarshalFrame(raw)
	if err != nil {
		t.Fatalf("UnmarshalFrame heartbeat ack: %v", err)
	}
	if ackFrame.Opcode != OpHeartbeatAck {
		t.Fatalf("unexpected heartbeat response opcode: %d", ackFrame.Opcode)
	}

	userTTL, err := rdb.TTL(ctx, userKey).Result()
	if err != nil {
		t.Fatalf("TTL user key: %v", err)
	}
	nodeTTL, err := rdb.TTL(ctx, nodeKey).Result()
	if err != nil {
		t.Fatalf("TTL node key: %v", err)
	}
	if userTTL < 4*time.Minute {
		t.Fatalf("user route ttl was not refreshed, got %s", userTTL)
	}
	if nodeTTL < 4*time.Minute {
		t.Fatalf("node route ttl was not refreshed, got %s", nodeTTL)
	}
}

type staticSyncSequenceProvider int64

func (p staticSyncSequenceProvider) LatestEventSeq(context.Context, int64) (int64, error) {
	return int64(p), nil
}

func newGatewayTestServer(manager *Manager) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", manager.HandleUpgrade)
	return httptest.NewServer(mux)
}

func mustHandshakeGatewayConn(t *testing.T, conn *websocket.Conn, accessToken, deviceID string) *livechat.HandshakeResponse {
	t.Helper()
	frame, err := NewFrame(OpHandshakeReq, &livechat.HandshakeRequest{
		AccessToken: accessToken,
		DeviceId:    deviceID,
		Platform:    "ios",
	})
	if err != nil {
		t.Fatalf("NewFrame handshake: %v", err)
	}
	raw, err := MarshalFrame(frame)
	if err != nil {
		t.Fatalf("MarshalFrame handshake: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, raw); err != nil {
		t.Fatalf("WriteMessage handshake: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline handshake response: %v", err)
	}
	_, raw, err = conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage handshake response: %v", err)
	}
	respFrame, err := UnmarshalFrame(raw)
	if err != nil {
		t.Fatalf("UnmarshalFrame handshake response: %v", err)
	}
	if respFrame.Opcode != OpHandshakeResp {
		t.Fatalf("unexpected handshake response opcode: %d", respFrame.Opcode)
	}
	resp := &livechat.HandshakeResponse{}
	if err := proto.Unmarshal(respFrame.Payload, resp); err != nil {
		t.Fatalf("proto.Unmarshal handshake response: %v", err)
	}
	if !resp.GetSuccess() {
		t.Fatalf("handshake was not successful")
	}
	if resp.GetSessionId() == "" {
		t.Fatalf("handshake response missing session_id")
	}
	if _, err := strconv.ParseInt(strings.Split(resp.GetSessionId(), ":")[0], 10, 64); err != nil {
		t.Fatalf("unexpected session id format: %s", resp.GetSessionId())
	}
	return resp
}
