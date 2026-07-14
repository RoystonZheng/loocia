// backfilltranslate is a one-off: it fills items.body_cn for English (rss)
// items that have a body but no translation yet, using the low-tier translate
// model. Idempotent, safe to re-run; only touches body_cn.
//
//	AIHOT_DATABASE_URL=postgres://... AIHOT_LLM_API_KEY=... [AIHOT_TRANSLATE_MODEL=...] ./backfilltranslate [-limit N]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"aihot-server/internal/db"
	"aihot-server/internal/llm"
	"aihot-server/internal/pipeline"
)

func main() {
	limit := flag.Int("limit", 1000, "max rss items to translate")
	flag.Parse()

	dsn := os.Getenv("AIHOT_DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "AIHOT_DATABASE_URL not set")
		os.Exit(1)
	}
	client, err := llm.NewTranslateClientFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "llm:", err)
		os.Exit(1)
	}
	tr := pipeline.NewTranslator(client, client.Model())

	ctx := context.Background()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "db:", err)
		os.Exit(1)
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, `
		SELECT id, body FROM items
		WHERE source_kind='rss' AND body IS NOT NULL AND body <> '' AND body_cn IS NULL
		ORDER BY COALESCE(published_at,'epoch'::timestamptz) DESC
		LIMIT $1`, *limit)
	if err != nil {
		fmt.Fprintln(os.Stderr, "query:", err)
		os.Exit(1)
	}
	type target struct{ id, body string }
	var targets []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.body); err != nil {
			rows.Close()
			fmt.Fprintln(os.Stderr, "scan:", err)
			os.Exit(1)
		}
		targets = append(targets, t)
	}
	rows.Close()

	var done, failed int
	for _, t := range targets {
		cn, err := tr.Translate(ctx, t.body)
		if err != nil || cn == "" {
			failed++
			fmt.Fprintf(os.Stderr, "translate %s: %v\n", t.id, err)
			continue
		}
		if _, err := pool.Exec(ctx, `UPDATE items SET body_cn=$1, updated_at=now() WHERE id=$2`, cn, t.id); err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "update %s: %v\n", t.id, err)
			continue
		}
		done++
	}
	fmt.Printf("backfilltranslate: targets=%d done=%d failed=%d\n", len(targets), done, failed)
}
