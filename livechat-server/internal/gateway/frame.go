package gateway

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"sync"

	"google.golang.org/protobuf/proto"
	livechat "github.com/tangzzz-fan/LiveChat/livechat-server/proto"
)

// Opcode constants from Spec 05 §3.2.
const (
	OpHandshakeReq  = 0x0001
	OpHandshakeResp = 0x0002
	OpHeartbeat     = 0x0003
	OpHeartbeatAck  = 0x0004
	OpAck           = 0x0005
	OpError         = 0x0006
	OpDisconnect    = 0x0007
	OpReconnect     = 0x0008

	OpMessageDelivery    = 0x1001
	OpMessageStatus      = 0x1002
	OpSyncEvent          = 0x2001
	OpConversationUpdate = 0x2002
)

// Protocol version.
const ProtocolVersion = 0x01

// OpPayload maps an opcode to a specific proto message factory.
// Returns a proto.Message that will be used to unmarshal the payload.
func UnmarshalPayload(opcode uint32, data []byte) (proto.Message, error) {
	var msg proto.Message
	switch opcode {
	case OpHandshakeReq:
		msg = &livechat.HandshakeRequest{}
	case OpHandshakeResp:
		msg = &livechat.HandshakeResponse{}
	case OpHeartbeat:
		msg = &livechat.Heartbeat{}
	case OpHeartbeatAck:
		msg = &livechat.HeartbeatAck{}
	case OpAck:
		msg = &livechat.MessageAck{}
	case OpError:
		msg = &livechat.ErrorFrame{}
	case OpMessageDelivery:
		msg = &livechat.WsMessageDelivery{}
	case OpSyncEvent:
		msg = &livechat.SyncEventMessage{}
	case OpConversationUpdate:
		msg = &livechat.ConversationUpdate{}
	default:
		return nil, fmt.Errorf("unknown opcode: 0x%04x", opcode)
	}
	if len(data) > 0 {
		if err := proto.Unmarshal(data, msg); err != nil {
			return nil, err
		}
	}
	return msg, nil
}

// MarshalFrame encodes a WsFrame into wire format.
// This uses the envelope approach where the entire frame is a protobuf.
func MarshalFrame(frame *livechat.WsFrame) ([]byte, error) {
	return proto.Marshal(frame)
}

// UnmarshalFrame decodes wire bytes into a WsFrame.
func UnmarshalFrame(data []byte) (*livechat.WsFrame, error) {
	frame := &livechat.WsFrame{}
	if err := proto.Unmarshal(data, frame); err != nil {
		return nil, err
	}
	return frame, nil
}

// ReadFrame reads a single protobuf frame from a reader using a 4-byte
// big-endian length prefix.
func ReadFrame(r io.Reader) (*livechat.WsFrame, error) {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	var lenBuf [4]byte
	if _, err := io.ReadFull(br, lenBuf[:]); err != nil {
		return nil, fmt.Errorf("read len: %w", err)
	}
	length := binary.BigEndian.Uint32(lenBuf[:])
	if length > 1024*1024 { // 1 MiB cap
		return nil, fmt.Errorf("frame too large: %d", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(br, data); err != nil {
		return nil, fmt.Errorf("read frame: %w", err)
	}
	return UnmarshalFrame(data)
}

// WriteFrame writes a single protobuf frame with a 4-byte length prefix.
func WriteFrame(w io.Writer, frame *livechat.WsFrame) error {
	raw, err := MarshalFrame(frame)
	if err != nil {
		return err
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(raw)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	return nil
}

// NewFrame creates a WsFrame with the given opcode and payload.
func NewFrame(opcode uint32, payload proto.Message) (*livechat.WsFrame, error) {
	raw, err := proto.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &livechat.WsFrame{
		Version: ProtocolVersion,
		Opcode:  opcode,
		Payload: raw,
	}, nil
}

// ── Frame write helper (thread-safe) ──────────────────

var writeMu sync.Mutex

func WriteFrameSafe(w io.Writer, opcode uint32, payload proto.Message) error {
	frame, err := NewFrame(opcode, payload)
	if err != nil {
		return err
	}
	writeMu.Lock()
	defer writeMu.Unlock()
	return WriteFrame(w, frame)
}
