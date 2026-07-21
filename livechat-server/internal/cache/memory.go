package cache

import (
	"context"
	"sync"
	"time"
)

// MemoryStore is an in-memory implementation of Store, intended for tests.
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]memoryEntry
}

type memoryEntry struct {
	value  []byte
	expiry time.Time
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]memoryEntry)}
}

func (s *MemoryStore) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.data[key]
	if !ok {
		return nil, ErrNotFound
	}
	if !entry.expiry.IsZero() && time.Now().After(entry.expiry) {
		return nil, ErrNotFound
	}
	val := make([]byte, len(entry.value))
	copy(val, entry.value)
	return val, nil
}

func (s *MemoryStore) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := memoryEntry{value: make([]byte, len(value))}
	copy(entry.value, value)
	if ttl > 0 {
		entry.expiry = time.Now().Add(ttl)
	}
	s.data[key] = entry
	return nil
}

func (s *MemoryStore) Del(_ context.Context, keys ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, k := range keys {
		delete(s.data, k)
	}
	return nil
}

func (s *MemoryStore) Exists(_ context.Context, key string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.data[key]
	if !ok {
		return false, nil
	}
	if !entry.expiry.IsZero() && time.Now().After(entry.expiry) {
		return false, nil
	}
	return true, nil
}
