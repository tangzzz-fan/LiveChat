package gateway

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// ConnectionLimiter provides Spec 05 §6.2 style admission control for new
// WebSocket handshakes: per-IP and per-user token buckets.
type ConnectionLimiter struct {
	mu sync.Mutex

	perIP   map[string]*tokenBucket
	perUser map[int64]*tokenBucket

	ipRate    float64 // tokens per second
	ipBurst   float64
	userRate  float64
	userBurst float64
}

func NewConnectionLimiter() *ConnectionLimiter {
	return &ConnectionLimiter{
		perIP:     make(map[string]*tokenBucket),
		perUser:   make(map[int64]*tokenBucket),
		ipRate:    5, // Spec: ≤5 new connections/s per IP
		ipBurst:   5,
		userRate:  2, // Spec: ≤2 new connections/s per user
		userBurst: 2,
	}
}

type tokenBucket struct {
	tokens   float64
	capacity float64
	rate     float64
	last     time.Time
}

func (b *tokenBucket) allow(now time.Time) bool {
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * b.rate
		if b.tokens > b.capacity {
			b.tokens = b.capacity
		}
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (l *ConnectionLimiter) bucket(m map[string]*tokenBucket, key string, rate, burst float64, now time.Time) *tokenBucket {
	b, ok := m[key]
	if !ok {
		b = &tokenBucket{tokens: burst, capacity: burst, rate: rate, last: now}
		m[key] = b
	}
	return b
}

func (l *ConnectionLimiter) bucketUser(m map[int64]*tokenBucket, key int64, rate, burst float64, now time.Time) *tokenBucket {
	b, ok := m[key]
	if !ok {
		b = &tokenBucket{tokens: burst, capacity: burst, rate: rate, last: now}
		m[key] = b
	}
	return b
}

// AllowIP returns false when the remote IP exceeds the new-connection budget.
func (l *ConnectionLimiter) AllowIP(r *http.Request) bool {
	if l == nil {
		return true
	}
	ip := clientIP(r)
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.bucket(l.perIP, ip, l.ipRate, l.ipBurst, now).allow(now)
}

// AllowUser returns false when the user exceeds the new-connection budget.
func (l *ConnectionLimiter) AllowUser(userID int64) bool {
	if l == nil {
		return true
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.bucketUser(l.perUser, userID, l.userRate, l.userBurst, now).allow(now)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
