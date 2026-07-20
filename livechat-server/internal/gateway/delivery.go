package gateway

import (
	"context"
	"errors"

	livechat "github.com/tangzzz-fan/LiveChat/livechat-server/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var ErrSessionNotFound = errors.New("gateway session not found")

// DeliverToDevice pushes a MESSAGE_DELIVERY frame to the target session.
func (m *Manager) DeliverToDevice(userID int64, deviceID string, msg *livechat.WsMessageDelivery) error {
	sess := m.GetSession(userID, deviceID)
	if sess == nil {
		return ErrSessionNotFound
	}
	return sess.Send(OpMessageDelivery, msg)
}

// DeliveryService exposes the spec-defined Gateway gRPC delivery endpoint.
type DeliveryService struct {
	livechat.UnimplementedGatewayDeliveryServiceServer
	manager *Manager
}

func NewDeliveryService(manager *Manager) *DeliveryService {
	return &DeliveryService{manager: manager}
}

func (s *DeliveryService) DeliverMessage(ctx context.Context, req *livechat.DeliverMessageRequest) (*livechat.DeliverMessageResponse, error) {
	if req.GetMessage() == nil {
		return nil, status.Error(codes.InvalidArgument, "message is required")
	}
	if req.GetDeviceId() == "" {
		return nil, status.Error(codes.InvalidArgument, "device_id is required")
	}
	if err := s.manager.DeliverToDevice(req.GetUserId(), req.GetDeviceId(), req.GetMessage()); err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			s.manager.unregisterRoute(context.Background(), req.GetUserId(), req.GetDeviceId())
			return nil, status.Error(codes.NotFound, "gateway session not found")
		}
		return nil, status.Errorf(codes.Internal, "deliver message: %v", err)
	}
	return &livechat.DeliverMessageResponse{}, nil
}
