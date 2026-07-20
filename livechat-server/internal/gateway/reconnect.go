package gateway

import (
	"math/rand"
	"time"
)

const fastReconnectThreshold = 5 * time.Minute

var reconnectRandFloat = rand.Float64

// ReconnectBackoffWindow returns the client-side reconnect delay window from Spec 05 §6.1.
func ReconnectBackoffWindow(attempt int) (time.Duration, time.Duration) {
	if attempt < 0 {
		attempt = 0
	}

	minDelay := 500 * time.Millisecond
	maxDelay := time.Second
	for i := 0; i < attempt; i++ {
		if minDelay < 16*time.Second {
			minDelay *= 2
			if minDelay > 16*time.Second {
				minDelay = 16 * time.Second
			}
		}
		if maxDelay < 30*time.Second {
			maxDelay *= 2
			if maxDelay > 30*time.Second {
				maxDelay = 30 * time.Second
			}
		}
	}
	return minDelay, maxDelay
}

// ReconnectBackoffDelay picks a concrete reconnect delay inside the bounded window.
func ReconnectBackoffDelay(attempt int) time.Duration {
	minDelay, maxDelay := ReconnectBackoffWindow(attempt)
	if maxDelay <= minDelay {
		return minDelay
	}
	return minDelay + time.Duration(reconnectRandFloat()*float64(maxDelay-minDelay))
}

// FastReconnectEligible indicates whether the previous connection lived long enough
// to try a quick reconnect before falling back to standard backoff.
func FastReconnectEligible(connectionAge time.Duration) bool {
	return connectionAge >= fastReconnectThreshold
}
