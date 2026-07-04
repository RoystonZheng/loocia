// gendaily generates (or regenerates) the daily report for one UTC day.
//
//	AIHOT_DATABASE_URL=postgres://... AIHOT_LLM_API_KEY=... \
//	  go run ./cmd/gendaily [-date 2026-05-07]
//
// Without AIHOT_LLM_API_KEY the report is generated with a nil lead.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"aihot-server/internal/daily"
	"aihot-server/internal/db"
	"aihot-server/internal/items"
	"aihot-server/internal/llm"
)

// noLLM makes gendaily usable without a key: lead generation just fails soft.
type noLLM struct{}

func (noLLM) Complete(ctx context.Context, system, user string) (string, error) {
	return "", fmt.Errorf("no AIHOT_LLM_API_KEY: lead skipped")
}

func main() {
	date := flag.String("date", time.Now().UTC().Format("2006-01-02"), "UTC day YYYY-MM-DD")
	flag.Parse()

	dsn := os.Getenv("AIHOT_DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "AIHOT_DATABASE_URL not set")
		os.Exit(1)
	}
	ctx := context.Background()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "db:", err)
		os.Exit(1)
	}
	defer pool.Close()

	store := daily.NewStore(pool)
	if err := store.EnsureSchema(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "schema:", err)
		os.Exit(1)
	}

	var model daily.LLM = noLLM{}
	if c, err := llm.NewClientFromEnv(); err == nil {
		model = c
	}

	g := daily.NewGenerator(items.New(pool), store, model)
	rep, err := g.Generate(ctx, *date, time.Now())
	if err != nil {
		fmt.Fprintln(os.Stderr, "generate:", err)
		os.Exit(1)
	}
	leadTitle := "(无导语)"
	if rep.Lead != nil {
		leadTitle = rep.Lead.Title
	}
	total := 0
	for _, s := range rep.Sections {
		total += len(s.Items)
	}
	fmt.Printf("daily %s: %d sections, %d items, %d flashes, lead=%q\n",
		rep.Date, len(rep.Sections), total, len(rep.Flashes), leadTitle)
}
