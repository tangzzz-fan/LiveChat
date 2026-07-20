package gateway

import "context"

// SyncSequenceProvider provides the latest per-user event sequence for handshake recovery decisions.
type SyncSequenceProvider interface {
	LatestEventSeq(ctx context.Context, userID int64) (int64, error)
}
