// importmp is a one-off: it reads the whole synced WeChat 公众号 corpus, keeps
// the AI-relevant articles, and InsertRaws them into raw_items with NO age gate,
// so the pulse enrich pipeline picks up the ~3-month backlog (steady-state
// ingest only takes articles <14d old). Idempotent — safe to re-run.
//
//	AIHOT_DATABASE_URL=postgres://... ./importmp -corpus /root/aihot/corpus/wechat
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"aihot-server/internal/db"
	"aihot-server/internal/ingest"
)

func main() {
	corpus := flag.String("corpus", "/root/aihot/corpus/wechat", "corpus root dir")
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

	raw := ingest.NewRawStore(pool)
	if err := raw.EnsureSchema(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "schema:", err)
		os.Exit(1)
	}

	src := ingest.NewMPCorpusSource(*corpus, "WeChat MP")
	items, err := src.Fetch(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fetch:", err)
		os.Exit(1)
	}
	inserted := 0
	for _, it := range items {
		ok, err := raw.InsertRaw(ctx, it)
		if err != nil {
			fmt.Fprintf(os.Stderr, "insert %s: %v\n", it.ID, err)
			continue
		}
		if ok {
			inserted++
		}
	}
	fmt.Printf("importmp: %d AI-relevant articles, %d newly inserted\n", len(items), inserted)
}
