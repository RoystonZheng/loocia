package pulse

import (
	"context"
	"fmt"

	"aihot-server/internal/ingest"
	"aihot-server/internal/items"
	"aihot-server/internal/pipeline"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Deps are the pulse unit's dependencies.
type Deps struct {
	Pool    *pgxpool.Pool
	LLM     pipeline.LLM
	Sources []ingest.Source
}

// Summary reports one pulse run.
type Summary struct {
	Fetched      int
	Inserted     int
	Skipped      int
	SourceErrors int
	Processed    int
	Failed       int
	Batches      int
}

const (
	batchSize  = 20
	maxBatches = 5 // hard cap: ≤100 enrichment calls per pulse
)

// Run executes one pulse: ensure schemas → ingest all sources → drain the
// enrichment queue in bounded batches. A batch that makes no progress
// (only failures) stops the drain so poison items can't spin the loop.
func Run(ctx context.Context, d Deps) (Summary, error) {
	var sum Summary

	rawStore := ingest.NewRawStore(d.Pool)
	if err := rawStore.EnsureSchema(ctx); err != nil {
		return sum, fmt.Errorf("raw schema: %w", err)
	}
	itemsStore := items.New(d.Pool)
	if err := itemsStore.EnsureSchema(ctx); err != nil {
		return sum, fmt.Errorf("items schema: %w", err)
	}

	res, err := ingest.NewRunner(rawStore, d.Sources...).RunOnce(ctx)
	if err != nil {
		return sum, fmt.Errorf("ingest: %w", err)
	}
	sum.Fetched, sum.Inserted, sum.Skipped = res.Fetched, res.Inserted, res.Skipped
	sum.SourceErrors = len(res.Errors)

	proc := pipeline.NewProcessor(rawStore, itemsStore, pipeline.NewEnricher(d.LLM))
	for sum.Batches < maxBatches {
		batch, err := proc.ProcessBatch(ctx, batchSize)
		if err != nil {
			return sum, fmt.Errorf("enrich batch: %w", err)
		}
		if batch.Processed+batch.Failed == 0 {
			break // queue drained
		}
		sum.Batches++
		sum.Processed += batch.Processed
		sum.Failed += batch.Failed
		if batch.Processed == 0 {
			break // no progress: only failures — stop, retry next pulse
		}
	}
	return sum, nil
}
