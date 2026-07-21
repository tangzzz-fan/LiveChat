package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConnectionLimiterPerIP(t *testing.T) {
	lim := NewConnectionLimiter()
	lim.ipBurst = 2
	lim.ipRate = 0.001 // essentially no refill during test

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.RemoteAddr = "10.0.0.1:12345"

	if !lim.AllowIP(req) {
		t.Fatal("first should allow")
	}
	if !lim.AllowIP(req) {
		t.Fatal("second should allow (burst=2)")
	}
	if lim.AllowIP(req) {
		t.Fatal("third should deny")
	}
}

func TestConnectionLimiterPerUser(t *testing.T) {
	lim := NewConnectionLimiter()
	lim.userBurst = 1
	lim.userRate = 0.001

	if !lim.AllowUser(42) {
		t.Fatal("first should allow")
	}
	if lim.AllowUser(42) {
		t.Fatal("second should deny")
	}
	if !lim.AllowUser(43) {
		t.Fatal("other user should allow")
	}
}
