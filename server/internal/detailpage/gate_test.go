package detailpage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aihot-server/internal/items"
)

// blockingTranslator blocks each Translate until released and counts calls.
type blockingTranslator struct {
	started chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (b *blockingTranslator) Translate(ctx context.Context, body string) (string, error) {
	b.calls.Add(1)
	close(b.started)
	<-b.release
	return "<p>中文</p>", nil
}

func (b *blockingTranslator) Model() string { return "auto-std" }

func TestRetranslateSingleflightSharesOneLLMCall(t *testing.T) {
	it := rssItem("abc")
	upd := &recordingUpdater{}
	tr := &blockingTranslator{started: make(chan struct{}), release: make(chan struct{})}
	h := NewHandler(fakeGetter{byID: map[string]*items.Item{"abc": it}}).WithRetranslate(tr, upd)

	codes := make([]int, 2)
	bodies := make([]string, 2)
	var wg sync.WaitGroup
	do := func(i int) {
		defer wg.Done()
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/items/abc/retranslate", nil))
		codes[i] = rr.Code
		bodies[i] = rr.Body.String()
	}
	wg.Add(2)
	go do(0)
	<-tr.started // leader is inside Translate
	go do(1)     // follower joins while leader is in flight
	time.Sleep(50 * time.Millisecond)
	close(tr.release)
	wg.Wait()

	if got := tr.calls.Load(); got != 1 {
		t.Fatalf("Translate called %d times, want 1", got)
	}
	for i := range codes {
		if codes[i] != http.StatusOK || bodies[i] != `{"ok":true}` {
			t.Fatalf("request %d: code=%d body=%q", i, codes[i], bodies[i])
		}
	}
}

func TestRetranslateRateLimit429(t *testing.T) {
	it := rssItem("abc")
	upd := &recordingUpdater{}
	tr := fakeTranslator{out: "<p>中文</p>", model: "auto-std"}
	h := NewHandler(fakeGetter{byID: map[string]*items.Item{"abc": it}}).WithRetranslate(tr, upd)
	h.gate = newRTGate(1, 0, time.Now()) // one token, no refill

	if rr := post(t, h, "/items/abc/retranslate"); rr.Code != http.StatusOK {
		t.Fatalf("first call: %d", rr.Code)
	}
	rr := post(t, h, "/items/abc/retranslate")
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("second call: %d, want 429", rr.Code)
	}
	if rr.Body.String() != `{"ok":false}` {
		t.Fatalf("body: %q", rr.Body.String())
	}
}

func TestRetranslateInvalidTargetsDoNotSpendTokens(t *testing.T) {
	it := rssItem("abc")
	upd := &recordingUpdater{}
	tr := fakeTranslator{out: "<p>中文</p>", model: "auto-std"}
	h := NewHandler(fakeGetter{byID: map[string]*items.Item{"abc": it}}).WithRetranslate(tr, upd)
	h.gate = newRTGate(1, 0, time.Now())

	if rr := post(t, h, "/items/nope/retranslate"); rr.Code != http.StatusNotFound {
		t.Fatalf("missing id: %d", rr.Code)
	}
	// The 404 above must not have consumed the only token.
	if rr := post(t, h, "/items/abc/retranslate"); rr.Code != http.StatusOK {
		t.Fatalf("valid call after 404: %d, want 200", rr.Code)
	}
}
