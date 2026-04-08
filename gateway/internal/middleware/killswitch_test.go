package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubKillChecker lets tests control IsKilled() without a real Redis connection.
// It satisfies the KillChecker interface.
type stubKillChecker struct{ killed bool }

func (s *stubKillChecker) IsKilled() bool { return s.killed }

var ksOKHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func TestKillSwitch_PassesThroughWhenInactive(t *testing.T) {
	mw := KillSwitch(&stubKillChecker{killed: false})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	mw(ksOKHandler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestKillSwitch_Returns503WhenActive(t *testing.T) {
	mw := KillSwitch(&stubKillChecker{killed: true})
	req := httptest.NewRequest(http.MethodGet, "/v1/feed", nil)
	w := httptest.NewRecorder()

	mw(ksOKHandler).ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestKillSwitch_BlocksHandlerWhenActive(t *testing.T) {
	reached := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
	})

	mw := KillSwitch(&stubKillChecker{killed: true})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mw(handler).ServeHTTP(w, req)

	if reached {
		t.Fatal("handler must not be called when kill switch is active")
	}
}

func TestKillSwitch_ReactivatesAfterToggle(t *testing.T) {
	stub := &stubKillChecker{killed: true}
	mw := KillSwitch(stub)
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	// Killed → 503.
	w := httptest.NewRecorder()
	mw(ksOKHandler).ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("killed: expected 503, got %d", w.Code)
	}

	// Resume → 200.
	stub.killed = false
	w = httptest.NewRecorder()
	mw(ksOKHandler).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("resumed: expected 200, got %d", w.Code)
	}
}
