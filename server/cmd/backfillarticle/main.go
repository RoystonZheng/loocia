// backfillarticle is a one-off, best-effort tool: for RSS items whose stored
// body is just a short feed snippet, it fetches the source-site page, extracts
// the full-article HTML, and replaces the body with it. It only ever touches the
// items.body column (UPDATE items SET body=..., updated_at=now()). It is safe to
// re-run: re-running simply re-fetches, and the betterBody guard prevents a
// longer body from being downgraded by a shorter/failed extraction.
//
//	AIHOT_DATABASE_URL=postgres://... ./backfillarticle [-limit N] [-sleep MS]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"aihot-server/internal/db"
	"aihot-server/internal/ingest"
)

var tagRe = regexp.MustCompile(`<[^>]+>`)

func textLen(s string) int {
	return len([]rune(strings.TrimSpace(tagRe.ReplaceAllString(s, ""))))
}

// betterBody: extracted article must be substantial (>=300 text chars) and
// clearly longer than the current snippet (>=1.5x) before we replace.
func betterBody(extracted, cur string) bool {
	n := textLen(extracted)
	return n >= 300 && n >= textLen(cur)*3/2
}

func main() {
	limit := flag.Int("limit", 500, "max rss items to consider")
	sleep := flag.Int("sleep", 300, "ms to sleep between fetches to be polite to source sites")
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

	rows, err := pool.Query(ctx, `
		SELECT id, url, body FROM items
		WHERE source_kind='rss' AND body IS NOT NULL
		  AND length(regexp_replace(body,'<[^>]+>','','g')) < 500
		ORDER BY COALESCE(published_at,'epoch'::timestamptz) DESC
		LIMIT $1`, *limit)
	if err != nil {
		fmt.Fprintln(os.Stderr, "query:", err)
		os.Exit(1)
	}
	type target struct{ id, url, body string }
	var targets []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.url, &t.body); err != nil {
			rows.Close()
			fmt.Fprintln(os.Stderr, "scan:", err)
			os.Exit(1)
		}
		targets = append(targets, t)
	}
	rows.Close()

	resolver := ingest.NewPageResolver()

	updated, skipped := 0, 0
	for i, t := range targets {
		_, _, article := resolver.Resolve(ctx, t.url)
		if article == nil || !betterBody(*article, t.body) {
			skipped++
		} else if _, err := pool.Exec(ctx,
			`UPDATE items SET body=$1, updated_at=now() WHERE id=$2`,
			*article, t.id); err != nil {
			skipped++
			fmt.Fprintf(os.Stderr, "update %s: %v\n", t.id, err)
		} else {
			updated++
		}
		if i < len(targets)-1 {
			time.Sleep(time.Duration(*sleep) * time.Millisecond)
		}
	}
	fmt.Printf("backfillarticle: %d targets, %d updated, %d skipped\n", len(targets), updated, skipped)
}
