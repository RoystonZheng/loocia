package detailpage

import (
	"sync"
	"time"
)

// rtResult is the JSON outcome of one retranslate attempt, shared with any
// duplicate concurrent requests for the same item.
type rtResult struct {
	ok   bool
	code int
}

type rtCall struct {
	done chan struct{}
	res  rtResult
}

// rtGate throttles POST /items/{id}/retranslate. The endpoint is reachable by
// anyone on the intranet and every leader request spends a call against the
// shared LLM key, so it needs both a per-item singleflight (double-clicks and
// races collapse into one translation) and a small global token bucket
// (sustained hammering gets 429 instead of draining the key's RPM budget).
type rtGate struct {
	mu       sync.Mutex
	inflight map[string]*rtCall
	tokens   float64
	burst    float64
	perSec   float64
	last     time.Time
}

func newRTGate(burst float64, perMinute float64, now time.Time) *rtGate {
	return &rtGate{
		inflight: make(map[string]*rtCall),
		tokens:   burst,
		burst:    burst,
		perSec:   perMinute / 60,
		last:     now,
	}
}

// begin registers a call for id. The second return is true for the leader —
// the caller that must do the work and finish with end(). Followers receive
// the in-flight call and must wait on its done channel.
func (g *rtGate) begin(id string) (*rtCall, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if c, ok := g.inflight[id]; ok {
		return c, false
	}
	c := &rtCall{done: make(chan struct{})}
	g.inflight[id] = c
	return c, true
}

// end publishes the leader's result and releases waiting followers.
func (g *rtGate) end(id string, res rtResult) {
	g.mu.Lock()
	c := g.inflight[id]
	delete(g.inflight, id)
	g.mu.Unlock()
	if c != nil {
		c.res = res
		close(c.done)
	}
}

// allow consumes one token if the bucket has one, refilling by elapsed time.
func (g *rtGate) allow(now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.tokens += now.Sub(g.last).Seconds() * g.perSec
	if g.tokens > g.burst {
		g.tokens = g.burst
	}
	g.last = now
	if g.tokens < 1 {
		return false
	}
	g.tokens--
	return true
}
