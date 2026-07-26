package backpressure

import (
	"testing"
	"time"
)

func newTestLimiter(threshold int64) *Limiter {
	return NewLimiter(nil, Config{
		PendingThreshold: threshold,
		RetryAfter:       3 * time.Second,
		SampleInterval:   time.Second,
	})
}

func TestAllowUnderThreshold(t *testing.T) {
	l := newTestLimiter(100)
	l.SetPending(99)

	allowed, retryAfter := l.Allow()
	if !allowed {
		t.Fatalf("expected send to be allowed at pending=99, threshold=100")
	}
	if retryAfter != 0 {
		t.Fatalf("expected no retry hint when allowed, got %v", retryAfter)
	}
	if l.Rejected() != 0 {
		t.Fatalf("expected rejected=0, got %d", l.Rejected())
	}
}

func TestRejectAtOrAboveThresholdThenRecover(t *testing.T) {
	l := newTestLimiter(100)
	l.SetPending(100)

	allowed, retryAfter := l.Allow()
	if allowed {
		t.Fatalf("expected send to be rejected at pending=100, threshold=100")
	}
	if retryAfter != 3*time.Second {
		t.Fatalf("expected retry_after=3s, got %v", retryAfter)
	}
	if l.Rejected() != 1 {
		t.Fatalf("expected rejected=1, got %d", l.Rejected())
	}

	// Consumer catches up -> sends resume without restarting the process.
	l.SetPending(10)
	if allowed, _ := l.Allow(); !allowed {
		t.Fatalf("expected send to be allowed again after backlog drained")
	}
	if l.Rejected() != 1 {
		t.Fatalf("rejected counter should not grow on success, got %d", l.Rejected())
	}
}

func TestDisabledWhenThresholdNonPositive(t *testing.T) {
	l := newTestLimiter(0)
	l.SetPending(1_000_000)

	if l.Enabled() {
		t.Fatalf("expected limiter to be disabled at threshold=0")
	}
	if allowed, _ := l.Allow(); !allowed {
		t.Fatalf("disabled limiter must always allow sends")
	}
}

func TestMetricsExposeThresholdAndSample(t *testing.T) {
	l := newTestLimiter(50)
	l.SetPending(70)
	l.Allow() // rejected

	m := l.Metrics()
	if m["send_backpressure_threshold"] != 50 {
		t.Fatalf("unexpected threshold metric: %d", m["send_backpressure_threshold"])
	}
	if m["send_backpressure_pending_sample"] != 70 {
		t.Fatalf("unexpected pending metric: %d", m["send_backpressure_pending_sample"])
	}
	if m["send_backpressure_rejected_total"] != 1 {
		t.Fatalf("unexpected rejected metric: %d", m["send_backpressure_rejected_total"])
	}
}

func TestConfigFromEnvOverrides(t *testing.T) {
	t.Setenv("SEND_BACKPRESSURE_PENDING_THRESHOLD", "42")
	t.Setenv("SEND_BACKPRESSURE_RETRY_AFTER_SEC", "7")
	t.Setenv("SEND_BACKPRESSURE_SAMPLE_MS", "250")

	cfg := ConfigFromEnv()
	if cfg.PendingThreshold != 42 {
		t.Fatalf("threshold: got %d", cfg.PendingThreshold)
	}
	if cfg.RetryAfter != 7*time.Second {
		t.Fatalf("retry after: got %v", cfg.RetryAfter)
	}
	if cfg.SampleInterval != 250*time.Millisecond {
		t.Fatalf("sample interval: got %v", cfg.SampleInterval)
	}
}

func TestConfigFromEnvIgnoresGarbage(t *testing.T) {
	t.Setenv("SEND_BACKPRESSURE_PENDING_THRESHOLD", "not-a-number")

	cfg := ConfigFromEnv()
	if cfg.PendingThreshold != DefaultPendingThreshold {
		t.Fatalf("expected default threshold on invalid env, got %d", cfg.PendingThreshold)
	}
}
