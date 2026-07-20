package gateway

import (
	"testing"
	"time"
)

func TestReconnectBackoffWindowGrowthAndCap(t *testing.T) {
	cases := []struct {
		attempt int
		wantMin time.Duration
		wantMax time.Duration
	}{
		{attempt: 0, wantMin: 500 * time.Millisecond, wantMax: time.Second},
		{attempt: 1, wantMin: time.Second, wantMax: 2 * time.Second},
		{attempt: 2, wantMin: 2 * time.Second, wantMax: 4 * time.Second},
		{attempt: 3, wantMin: 4 * time.Second, wantMax: 8 * time.Second},
		{attempt: 4, wantMin: 8 * time.Second, wantMax: 16 * time.Second},
		{attempt: 5, wantMin: 16 * time.Second, wantMax: 30 * time.Second},
		{attempt: 8, wantMin: 16 * time.Second, wantMax: 30 * time.Second},
	}

	for _, tc := range cases {
		gotMin, gotMax := ReconnectBackoffWindow(tc.attempt)
		if gotMin != tc.wantMin || gotMax != tc.wantMax {
			t.Fatalf("attempt=%d: want [%s,%s], got [%s,%s]", tc.attempt, tc.wantMin, tc.wantMax, gotMin, gotMax)
		}
	}
}

func TestReconnectBackoffDelayStaysInsideWindow(t *testing.T) {
	originalRand := reconnectRandFloat
	t.Cleanup(func() { reconnectRandFloat = originalRand })

	reconnectRandFloat = func() float64 { return 0 }
	if got := ReconnectBackoffDelay(3); got != 4*time.Second {
		t.Fatalf("expected min delay for attempt 3, got %s", got)
	}

	reconnectRandFloat = func() float64 { return 0.999999 }
	got := ReconnectBackoffDelay(5)
	if got < 16*time.Second || got >= 30*time.Second {
		t.Fatalf("expected capped jitter delay in [16s,30s), got %s", got)
	}
}

func TestFastReconnectEligible(t *testing.T) {
	if FastReconnectEligible(4*time.Minute + 59*time.Second) {
		t.Fatalf("expected connection younger than 5m to skip fast reconnect")
	}
	if !FastReconnectEligible(5 * time.Minute) {
		t.Fatalf("expected connection at 5m to allow fast reconnect")
	}
	if !FastReconnectEligible(12 * time.Minute) {
		t.Fatalf("expected long-lived connection to allow fast reconnect")
	}
}
