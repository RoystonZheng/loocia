package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(ctx context.Context) error { return f.err }

func TestHealthzOKWhenDBUp(t *testing.T) {
	h := NewHandler(fakePinger{err: nil})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	if got := strings.TrimSpace(rr.Body.String()); got != `{"status":"ok"}` {
		t.Fatalf("want body {\"status\":\"ok\"}, got %s", got)
	}
}

func TestHealthz503WhenDBDown(t *testing.T) {
	h := NewHandler(fakePinger{err: context.DeadlineExceeded})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rr.Code)
	}
	if got := strings.TrimSpace(rr.Body.String()); got != `{"status":"down"}` {
		t.Fatalf("want body {\"status\":\"down\"}, got %s", got)
	}
}
