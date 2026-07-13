// backfillreason is a one-off: it fills the 精选理由 (items.reason) for already-
// enriched SELECTED items that lack one, via a focused one-sentence LLM call over
// the item's existing title + summary. Non-selected items are left alone (their
// reason only shows on the 精选/detail surfaces). Idempotent, read-mostly, safe
// to re-run; only touches the reason column.
//
//	AIHOT_DATABASE_URL=postgres://... AIHOT_LLM_API_KEY=... ./backfillreason [-limit N]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"aihot-server/internal/db"
	"aihot-server/internal/llm"
)

const reasonSystemPrompt = `你是 AI 资讯编辑。给定一条资讯的中文标题和摘要，用一句话（不超过40字）说明它为什么值得精选/关注，点出关键看点，不要复述标题。只输出这句话本身，不要引号、不要解释。`

func main() {
	limit := flag.Int("limit", 1000, "max selected items to backfill")
	flag.Parse()

	dsn := os.Getenv("AIHOT_DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "AIHOT_DATABASE_URL not set")
		os.Exit(1)
	}
	client, err := llm.NewClientFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "llm:", err)
		os.Exit(1)
	}
	ctx := context.Background()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "db:", err)
		os.Exit(1)
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, `
		SELECT id, title, COALESCE(summary,'') FROM items
		WHERE selected = true AND present = true AND reason IS NULL
		ORDER BY COALESCE(published_at,'epoch'::timestamptz) DESC
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

	filled, failed := 0, 0
	for _, t := range targets {
		user := "标题：" + t.title
		if t.summary != "" {
			user += "\n摘要：" + t.summary
		}
		out, err := client.Complete(ctx, reasonSystemPrompt, user)
		if err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "reason %s: %v\n", t.id, err)
			continue
		}
		reason := strings.TrimSpace(strings.Trim(strings.TrimSpace(out), `"'「」`))
		if reason == "" {
			failed++
			continue
		}
		if _, err := pool.Exec(ctx,
			`UPDATE items SET reason=$1, updated_at=now() WHERE id=$2 AND reason IS NULL`,
			reason, t.id); err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "update %s: %v\n", t.id, err)
			continue
		}
		filled++
	}
	fmt.Printf("backfillreason: %d selected items, %d filled, %d failed\n", len(targets), filled, failed)
}
