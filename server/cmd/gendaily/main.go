// gendaily generates (or regenerates) the daily report for one UTC day.
//
//	AIHOT_DATABASE_URL=postgres://... AIHOT_LLM_API_KEY=... \
//	  go run ./cmd/gendaily [-date 2026-05-07]
//
// Without AIHOT_LLM_API_KEY the report is generated with a nil lead.
package main

import (
	"context"
	"errors"
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
	// Empty default → regenerate yesterday and today: low-volume feeds rarely
	// publish enough on the current UTC day to fill a digest, and a "daily" is
	// naturally the completed previous day. An explicit -date generates just that
	// one day (backfill/regen). Per-day emptiness is skipped, not a failure.
	date := flag.String("date", "", "UTC day YYYY-MM-DD (default: regenerate yesterday and today)")
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

	var days []string
	explicit := *date != ""
	if explicit {
		days = []string{*date}
	} else {
		now := time.Now().UTC()
		days = []string{now.AddDate(0, 0, -1).Format("2006-01-02"), now.Format("2006-01-02")}
	}

	generated := 0
	for _, d := range days {
		rep, err := g.Generate(ctx, d, time.Now())
		if err != nil {
			if errors.Is(err, daily.ErrNoItems) {
				fmt.Printf("daily %s: no selected items, skipped\n", d)
				continue
			}
			fmt.Fprintln(os.Stderr, "generate:", err)
			os.Exit(1)
		}
		generated++
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

	// An explicit single-day request that produced nothing is a real error
	// (backfill callers want the non-zero exit); the default multi-day sweep
	// tolerates empty days silently.
	if explicit && generated == 0 {
		os.Exit(1)
	}
}
