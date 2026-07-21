package cache

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound indicates the key has not been set.
var ErrNotFound = errors.New("cache: key not found")

// ErrNotAvailable indicates the cache backend is degraded.
var ErrNotAvailable = errors.New("cache: backend not available")

// Store is the generic cache interface.
type Store interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, key string) (bool, error)
}

// StringStore is a convenience wrapper around Store for string values.
type StringStore struct {
	store Store
}

func NewStringStore(s Store) *StringStore {
	return &StringStore{store: s}
}

func (s *StringStore) Get(ctx context.Context, key string) (string, error) {
	b, err := s.store.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (s *StringStore) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return s.store.Set(ctx, key, []byte(value), ttl)
}

func (s *StringStore) Del(ctx context.Context, keys ...string) error {
	return s.store.Del(ctx, keys...)
}
