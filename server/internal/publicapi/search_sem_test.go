package publicapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"aihot-server/internal/items"
)

// gatedLister blocks List calls until released, so tests can hold a search
// in flight while probing the semaphore.
type gatedLister struct {
	entered chan struct{}
	release chan struct{}
}

func (g *gatedLister) List(ctx context.Context, p items.ListParams) ([]items.Item, error) {
	g.entered <- struct{}{}
	select {
	case <-g.release:
	case <-ctx.Done():
	}
	return nil, nil
}

func TestSearchSemaphoreQueuesAndRejectsOnCancel(t *testing.T) {
	gl := &gatedLister{entered: make(chan struct{}, 4), release: make(chan struct{})}
	h := NewItemsHandler(gl, time.Now)
	h.searchSem = make(chan struct{}, 1) // shrink for the test

	// First search occupies the only slot.
	done1 := make(chan int, 1)
	go func() {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/items?q=模型", nil))
		done1 <- rr.Code
	}()
	<-gl.entered

	// Second search with an already-canceled context can't take a slot → 503.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, httptest.NewRequest(http.MethodGet, "/api/public/items?q=芯片", nil).WithContext(ctx))
	if rr2.Code != http.StatusServiceUnavailable {
		t.Fatalf("queued search with dead ctx: %d, want 503", rr2.Code)
	}

	// A non-search request bypasses the semaphore entirely.
	done3 := make(chan int, 1)
	go func() {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/items", nil))
		done3 <- rr.Code
	}()
	select {
	case <-gl.entered: // reached the store while the search slot is still held
	case <-time.After(2 * time.Second):
		t.Fatal("non-search request blocked behind search semaphore")
	}

	close(gl.release)
	if code := <-done1; code != http.StatusOK {
		t.Fatalf("first search: %d", code)
	}
	if code := <-done3; code != http.StatusOK {
		t.Fatalf("browse request: %d", code)
	}
}
