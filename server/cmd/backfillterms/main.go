// backfillterms is a one-off: it extracts entities/topics into item_terms for
// items that don't have any yet, using the low-tier terms model. Idempotent,
// safe to re-run; only touches item_terms.
//
//	AIHOT_DATABASE_URL=postgres://... AIHOT_LLM_API_KEY=... [AIHOT_TERMS_MODEL=...] ./backfillterms [-limit N]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"aihot-server/internal/db"
	"aihot-server/internal/llm"
	"aihot-server/internal/pipeline"
	"aihot-server/internal/terms"
)

func main() {
	limit := flag.Int("limit", 2000, "max items to extract terms for")
	flag.Parse()

	dsn := os.Getenv("AIHOT_DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "AIHOT_DATABASE_URL not set")
		os.Exit(1)
	}
	client, err := llm.NewTermsClientFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "llm:", err)
		os.Exit(1)
	}
	x := pipeline.NewTermExtractor(client)

	ctx := context.Background()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "db:", err)
		os.Exit(1)
	}
	defer pool.Close()
	store := terms.NewStore(pool)
	if err := store.EnsureSchema(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "schema:", err)
		os.Exit(1)
	}

	rows, err := pool.Query(ctx, `
		SELECT i.id, i.title, COALESCE(i.summary, '')
		FROM items i
		WHERE NOT EXISTS (SELECT 1 FROM item_terms t WHERE t.item_id = i.id)
		ORDER BY COALESCE(i.published_at,'epoch'::timestamptz) DESC
		LIMIT $1`, *limit)
	if err != nil {
		fmt.Fprintln(os.Stderr, "query:", err)
		os.Exit(1)
	}
	type target struct{ id, title, summary string }
	var targets []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.title, &t.summary); err != nil {
			rows.Close()
			fmt.Fprintln(os.Stderr, "scan:", err)
			os.Exit(1)
		}
		targets = append(targets, t)
	}
	rows.Close()

	var done, empty, failed int
	for i, t := range targets {
		ts, err := x.Extract(ctx, t.title, t.summary)
		if err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "extract %s: %v\n", t.id, err)
			continue
		}
		rws := pipeline.TermRowsForBackfill(ts)
		if len(rws) == 0 {
			empty++
			continue
		}
		if err := store.ReplaceForItem(ctx, t.id, rws); err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "store %s: %v\n", t.id, err)
			continue
		}
		done++
		if (i+1)%25 == 0 {
			fmt.Printf("progress: %d/%d done=%d empty=%d failed=%d\n", i+1, len(targets), done, empty, failed)
		}
	}
	fmt.Printf("backfillterms: targets=%d done=%d empty=%d failed=%d\n", len(targets), done, empty, failed)
}
