package ingest

import (
	"context"
	"fmt"
	"time"
)

// Result summarizes one ingestion pass.
type Result struct {
	Fetched    int
	Inserted   int
	Skipped    int
	AgeSkipped int
	Errors     []error
}

// Runner fetches from all sources and inserts into the raw store.
type Runner struct {
	store   *RawStore
	sources []Source

	// MaxAge, when >0, skips items published more than MaxAge before now (nil PublishedAt passes). Guards against full-archive feeds.
	MaxAge time.Duration
}

func NewRunner(store *RawStore, sources ...Source) *Runner {
	return &Runner{store: store, sources: sources}
}

// RunOnce fetches every source once and inserts its items. A single source's
// failure is collected into Result.Errors and does not abort the others; the
// method returns a top-level error only on an unexpected store failure.
func (r *Runner) RunOnce(ctx context.Context) (Result, error) {
	var res Result
	for _, src := range r.sources {
		items, err := src.Fetch(ctx)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("source %q: %w", src.Name(), err))
			continue
		}
		for _, it := range items {
			res.Fetched++
			if r.MaxAge > 0 && it.PublishedAt != nil && time.Since(*it.PublishedAt) > r.MaxAge {
				res.AgeSkipped++
				continue
			}
			inserted, err := r.store.InsertRaw(ctx, it)
			if err != nil {
				return res, err
			}
			if inserted {
				res.Inserted++
			} else {
				res.Skipped++
			}
		}
	}
	return res, nil
}
