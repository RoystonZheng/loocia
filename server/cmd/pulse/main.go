// pulse runs one ingest+enrich cycle: fetch all RSS sources, then drain the
// enrichment queue in bounded batches.
//
//	AIHOT_DATABASE_URL=postgres://... AIHOT_LLM_API_KEY=... \
//	  go run ./cmd/pulse [-sources /path/sources.json]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"aihot-server/internal/db"
	"aihot-server/internal/llm"
	"aihot-server/internal/pipeline"
	"aihot-server/internal/pulse"
)

func main() {
	sourcesPath := flag.String("sources", "", "optional JSON sources file (default: embedded list)")
	ingestOnly := flag.Bool("ingest-only", os.Getenv("AIHOT_PULSE_INGEST_ONLY") == "1", "fetch and persist raw items without LLM enrichment")
	flag.Parse()

	dsn := os.Getenv("AIHOT_DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "AIHOT_DATABASE_URL not set")
		os.Exit(1)
	}
	sources, err := pulse.LoadSources(*sourcesPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sources:", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "db:", err)
		os.Exit(1)
	}
	defer pool.Close()

	if *ingestOnly {
		sum, err := pulse.RunIngestOnly(ctx, pool, sources)
		if err != nil {
			fmt.Fprintln(os.Stderr, "run:", err)
			os.Exit(1)
		}
		fmt.Printf("pulse ingest-only: fetched=%d inserted=%d skipped=%d ageskip=%d srcerrs=%d\n",
			sum.Fetched, sum.Inserted, sum.Skipped, sum.AgeSkipped, sum.SourceErrors)
		for _, sourceErr := range sum.SourceErrorDetails {
			fmt.Fprintln(os.Stderr, "source error:", sourceErr)
		}
		if err := pulseSummaryError(sum); err != nil {
			fmt.Fprintln(os.Stderr, "run:", err)
			os.Exit(1)
		}
		return
	}

	client, err := llm.NewClientFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "llm:", err)
		os.Exit(1)
	}
	translateClient, err := llm.NewTranslateClientFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "translate llm:", err)
		os.Exit(1)
	}
	termsClient, err := llm.NewTermsClientFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "terms llm:", err)
		os.Exit(1)
	}

	sum, err := pulse.Run(ctx, pulse.Deps{
		Pool:       pool,
		LLM:        client,
		Translator: pipeline.NewTranslator(translateClient, translateClient.Model()),
		Terms:      pipeline.NewTermExtractor(termsClient),
		Sources:    sources,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "run:", err)
		os.Exit(1)
	}
	fmt.Printf("pulse: fetched=%d inserted=%d skipped=%d ageskip=%d srcerrs=%d enriched=%d failed=%d batches=%d\n",
		sum.Fetched, sum.Inserted, sum.Skipped, sum.AgeSkipped, sum.SourceErrors, sum.Processed, sum.Failed, sum.Batches)
	for _, sourceErr := range sum.SourceErrorDetails {
		fmt.Fprintln(os.Stderr, "source error:", sourceErr)
	}
	if err := pulseSummaryError(sum); err != nil {
		fmt.Fprintln(os.Stderr, "run:", err)
		os.Exit(1)
	}
}

func pulseSummaryError(sum pulse.Summary) error {
	if sum.SourceErrors == 0 && sum.Failed == 0 {
		return nil
	}
	return fmt.Errorf("pulse completed with source_errors=%d failed=%d", sum.SourceErrors, sum.Failed)
}
