package receipts

import (
	"context"

	livechat "github.com/tangzzz-fan/LiveChat/livechat-server/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type GRPCServer struct {
	livechat.UnimplementedMessageAckServiceServer
	service *Service
}

func NewGRPCServer(service *Service) *GRPCServer {
	return &GRPCServer{service: service}
}

func (s *GRPCServer) ProcessAck(ctx context.Context, req *livechat.ProcessAckRequest) (*livechat.ProcessAckResponse, error) {
	err := s.service.ProcessAck(ctx, AckRequest{
		UserID:         req.GetUserId(),
		DeviceID:       req.GetDeviceId(),
		AckType:        req.GetAckType(),
		EventSeq:       int64(req.GetEventSeq()),
		ConversationID: req.GetConversationId(),
		LastReadSeq:    int64(req.GetLastReadSeq()),
		AckedAtMs:      NormalizeAckedAt(int64(req.GetAckedAtMs())),
		TraceID:        req.GetTraceId(),
	})
	if err != nil {
		if err == ErrUnsupportedAckType {
			return nil, status.Error(codes.InvalidArgument, "unsupported ack type")
		}
		return nil, status.Errorf(codes.Internal, "process ack: %v", err)
	}
	return &livechat.ProcessAckResponse{Accepted: true}, nil
}
