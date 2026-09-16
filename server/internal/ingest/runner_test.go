package ingest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeSource yields fixed items (or an error) for testing the runner.
type fakeSource struct {
	name  string
	items []RawItem
	err   error
}

func (f fakeSource) Name() string { return f.name }
func (f fakeSource) Fetch(ctx context.Context) ([]RawItem, error) {
	return f.items, f.err
}

func TestRunOnceInsertsAndIsIdempotent(t *testing.T) {
	s := newTestRawStore(t)
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	good := fakeSource{name: "good", items: []RawItem{
		sampleRaw("https://ex.com/a", base),
		sampleRaw("https://ex.com/b", base.Add(time.Hour)),
	}}

	r := NewRunner(s, good)
	res, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if res.Fetched != 2 || res.Inserted != 2 || res.Skipped != 0 {
		t.Fatalf("first run: %+v", res)
	}

	// Second run: same items already present → all skipped, none inserted.
	res2, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce 2: %v", err)
	}
	if res2.Inserted != 0 || res2.Skipped != 2 {
		t.Fatalf("second run should skip all: %+v", res2)
	}
}

func TestRunOnceContinuesWhenOneSourceFails(t *testing.T) {
	s := newTestRawStore(t)
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	bad := fakeSource{name: "bad", err: errors.New("boom")}
	good := fakeSource{name: "good", items: []RawItem{sampleRaw("https://ex.com/a", base)}}

	r := NewRunner(s, bad, good)
	res, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce should not return a top-level error: %v", err)
	}
	if res.Inserted != 1 {
		t.Fatalf("good source should still insert 1: %+v", res)
	}
	if len(res.Errors) != 1 {
		t.Fatalf("want 1 collected source error, got %d", len(res.Errors))
	}
	if !strings.Contains(res.Errors[0].Error(), "bad") {
		t.Fatalf("error should name the failing source, got %v", res.Errors[0])
	}
}

func TestRunOnceMaxAgeSkipsOldItems(t *testing.T) {
	s := newTestRawStore(t)
	fresh := sampleRaw("https://ex.com/fresh", time.Now().UTC().Add(-time.Hour))
	old := sampleRaw("https://ex.com/old", time.Now().UTC().Add(-30*24*time.Hour))
	src := fakeSource{name: "s", items: []RawItem{fresh, old}}

	r := NewRunner(s, src)
	r.MaxAge = 14 * 24 * time.Hour
	res, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if res.Inserted != 1 || res.AgeSkipped != 1 {
		t.Fatalf("age filter: %+v", res)
	}
	un, err := s.ListUnprocessed(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(un) != 1 || un[0].ID != fresh.ID {
		t.Fatalf("only fresh should be queued: %+v", un)
	}
}

func TestRateLimitedSourceWaitsBeforeFetch(t *testing.T) {
	delay := 20 * time.Millisecond
	src := NewRateLimitedSource(fakeSource{name: "limited"}, delay)

	started := time.Now()
	if _, err := src.Fetch(context.Background()); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if elapsed := time.Since(started); elapsed < delay {
		t.Fatalf("Fetch returned before delay elapsed: got %s want >= %s", elapsed, delay)
	}
}

func TestRateLimitedSourceHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	src := NewRateLimitedSource(fakeSource{name: "limited"}, time.Hour)
	_, err := src.Fetch(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Fetch should return context cancellation, got %v", err)
	}
}
