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
