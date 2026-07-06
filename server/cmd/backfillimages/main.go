// backfillimages is a one-off: it re-fetches the configured sources and fills
// image_url on already-stored items that don't have one yet. It only touches
// image_url (no re-enrichment, no cluster/selected changes), so it is safe to
// run against production. New items get their image at ingest time; this is only
// needed to seed items that were stored before image extraction existed.
//
//	AIHOT_DATABASE_URL=postgres://... go run ./cmd/backfillimages [-sources path.json]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"aihot-server/internal/db"
	"aihot-server/internal/pulse"
)

func main() {
	sourcesPath := flag.String("sources", "", "optional sources JSON (default: built-in)")
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

	sources, err := pulse.LoadSources(*sourcesPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sources:", err)
		os.Exit(1)
	}

	total, updated := 0, 0
	for _, src := range sources {
		raws, err := src.Fetch(ctx)
		if err != nil {
			fmt.Printf("fetch %s: %v (skipped)\n", src.Name(), err)
			continue
		}
		for _, r := range raws {
			if r.ImageURL == nil {
				continue
			}
			total++
			tag, err := pool.Exec(ctx,
				`UPDATE items SET image_url=$1, updated_at=now() WHERE id=$2 AND image_url IS NULL`,
				*r.ImageURL, r.ID)
			if err != nil {
				fmt.Printf("update %s: %v\n", r.ID, err)
				continue
			}
			updated += int(tag.RowsAffected())
		}
	}
	fmt.Printf("backfill: %d feed items with images, %d existing rows filled\n", total, updated)
}
