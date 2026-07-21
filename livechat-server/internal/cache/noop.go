package cache

import (
	"context"
	"time"
)

// NoopStore is a no-op implementation of Store that always returns miss.
// Used when the cache backend is unavailable.
type NoopStore struct{}

func NewNoopStore() *NoopStore {
	return &NoopStore{}
}

func (s *NoopStore) Get(_ context.Context, _ string) ([]byte, error) {
	return nil, ErrNotFound
}

func (s *NoopStore) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error {
	return nil
}

func (s *NoopStore) Del(_ context.Context, _ ...string) error {
	return nil
}

func (s *NoopStore) Exists(_ context.Context, _ string) (bool, error) {
	return false, nil
}
