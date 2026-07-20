package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tangzzz-fan/LiveChat/livechat-server/internal/gateway"
	livechat "github.com/tangzzz-fan/LiveChat/livechat-server/proto"
	"google.golang.org/protobuf/proto"
)

func main() {
	var (
		mode           = flag.String("mode", "delivery", "delivery or delivery-read-ack")
		wsURL          = flag.String("ws-url", "ws://localhost:8081/ws", "gateway websocket url")
		token          = flag.String("token", "", "access token")
		deviceID       = flag.String("device-id", "", "device id")
		readyFile      = flag.String("ready-file", "", "file created after handshake")
		outputFile     = flag.String("output-file", "", "json output file")
		targetSeq      = flag.Uint64("target-seq", 1, "wait until this conversation seq is delivered")
		ackLastReadSeq = flag.Uint64("ack-last-read-seq", 0, "override last_read_seq in ACK; default uses delivered seq")
		ackTraceID     = flag.String("ack-trace-id", "", "trace id used on outbound ACK frame")
	)
	flag.Parse()

	if *token == "" || *deviceID == "" || *outputFile == "" {
		exitf("token, device-id and output-file are required")
	}

	conn, _, err := websocket.DefaultDialer.Dial(*wsURL, nil)
	if err != nil {
		exitf("dial websocket: %v", err)
	}
	defer conn.Close()

	if err := handshake(conn, *token, *deviceID); err != nil {
		exitf("handshake: %v", err)
	}
	if *readyFile != "" {
		if err := os.WriteFile(*readyFile, []byte("ready\n"), 0o644); err != nil {
			exitf("write ready file: %v", err)
		}
	}

	for {
		if err := conn.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
			exitf("set read deadline: %v", err)
		}
		_, raw, err := conn.ReadMessage()
		if err != nil {
			exitf("read message: %v", err)
		}
		frame, err := gateway.UnmarshalFrame(raw)
		if err != nil {
			exitf("unmarshal frame: %v", err)
		}
		if frame.GetOpcode() != gateway.OpMessageDelivery {
			continue
		}
		delivery := &livechat.WsMessageDelivery{}
		if err := proto.Unmarshal(frame.GetPayload(), delivery); err != nil {
			exitf("unmarshal delivery payload: %v", err)
		}
		if delivery.GetConversationSeq() < *targetSeq {
			continue
		}

		lastReadSeq := delivery.GetConversationSeq()
		if *ackLastReadSeq > 0 {
			lastReadSeq = *ackLastReadSeq
		}
		if *mode == "delivery-read-ack" {
			if err := sendReadAck(conn, delivery, lastReadSeq, *ackTraceID); err != nil {
				exitf("send read ack: %v", err)
			}
		}

		result := map[string]any{
			"frame_trace_id":    frame.GetTraceId(),
			"ack_trace_id":      *ackTraceID,
			"conversation_id":   delivery.GetConversationId(),
			"conversation_seq":  delivery.GetConversationSeq(),
			"server_message_id": delivery.GetServerMessageId(),
			"content":           delivery.GetContent(),
			"last_read_seq":     lastReadSeq,
		}
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			exitf("marshal output: %v", err)
		}
		if err := os.WriteFile(*outputFile, data, 0o644); err != nil {
			exitf("write output: %v", err)
		}
		return
	}
}

func handshake(conn *websocket.Conn, token, deviceID string) error {
	frame, err := gateway.NewFrame(gateway.OpHandshakeReq, &livechat.HandshakeRequest{
		AccessToken: token,
		DeviceId:    deviceID,
		Platform:    "ios",
	})
	if err != nil {
		return err
	}
	raw, err := gateway.MarshalFrame(frame)
	if err != nil {
		return err
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, raw); err != nil {
		return err
	}
	if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	_, raw, err = conn.ReadMessage()
	if err != nil {
		return err
	}
	respFrame, err := gateway.UnmarshalFrame(raw)
	if err != nil {
		return err
	}
	if respFrame.GetOpcode() != gateway.OpHandshakeResp {
		return fmt.Errorf("unexpected handshake opcode: %d", respFrame.GetOpcode())
	}
	resp := &livechat.HandshakeResponse{}
	if err := proto.Unmarshal(respFrame.GetPayload(), resp); err != nil {
		return err
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("handshake failed: %s", resp.GetErrorMessage())
	}
	return nil
}

func sendReadAck(conn *websocket.Conn, delivery *livechat.WsMessageDelivery, lastReadSeq uint64, traceID string) error {
	frame, err := gateway.NewFrameWithTrace(gateway.OpAck, &livechat.MessageAck{
		AckType:        "read",
		EventSeq:       delivery.GetConversationSeq(),
		AckedAtMs:      uint64(time.Now().UnixMilli()),
		ConversationId: delivery.GetConversationId(),
		LastReadSeq:    lastReadSeq,
	}, traceID)
	if err != nil {
		return err
	}
	raw, err := gateway.MarshalFrame(frame)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, raw)
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
