// backfillimages is a one-off: it fills image_url / video_url on already-stored
// items that don't have them yet. It runs two passes:
//
//  1. Feed pass — re-fetches the configured sources and copies any image/video
//     found in the feed onto matching items (only fills NULL columns).
//  2. OG pass — for items still missing an image, fetches the article page and
//     reads its Open Graph image/video tags.
//
// It only touches image_url/video_url (no re-enrichment, no cluster/selected
// changes), so it is safe to run against production. New items get their media
// at ingest time; this seeds items stored before media extraction existed.
//
//	AIHOT_DATABASE_URL=postgres://... go run ./cmd/backfillimages [-sources path.json] [-og-limit N]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"aihot-server/internal/db"
	"aihot-server/internal/ingest"
	"aihot-server/internal/pulse"
)

func main() {
	sourcesPath := flag.String("sources", "", "optional sources JSON (default: built-in)")
	ogLimit := flag.Int("og-limit", 400, "max items to OG-resolve in the fallback pass (0 disables)")
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

	// Pass 1: feed media.
	sources, err := pulse.LoadSources(*sourcesPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sources:", err)
		os.Exit(1)
	}
	feedImg, feedVid := 0, 0
	for _, src := range sources {
		raws, err := src.Fetch(ctx)
		if err != nil {
			fmt.Printf("fetch %s: %v (skipped)\n", src.Name(), err)
			continue
		}
		for _, r := range raws {
			if r.ImageURL != nil {
				tag, err := pool.Exec(ctx,
					`UPDATE items SET image_url=$1, updated_at=now() WHERE id=$2 AND image_url IS NULL`,
					*r.ImageURL, r.ID)
				if err == nil {
					feedImg += int(tag.RowsAffected())
				}
			}
			if r.VideoURL != nil {
				tag, err := pool.Exec(ctx,
					`UPDATE items SET video_url=$1, updated_at=now() WHERE id=$2 AND video_url IS NULL`,
					*r.VideoURL, r.ID)
				if err == nil {
					feedVid += int(tag.RowsAffected())
				}
			}
		}
	}
	fmt.Printf("feed pass: %d images, %d videos filled\n", feedImg, feedVid)

	// Pass 2: Open Graph fallback for items still lacking an image.
	if *ogLimit > 0 {
		ogImg, ogVid := ogBackfill(ctx, pool, *ogLimit)
		fmt.Printf("og pass: %d images, %d videos filled\n", ogImg, ogVid)
	}
}

// ogBackfill fetches article pages for image-less items and fills their media
// from Open Graph tags. Bounded by limit (newest items first).
func ogBackfill(ctx context.Context, pool *pgxpool.Pool, limit int) (int, int) {
	rows, err := pool.Query(ctx, `
		SELECT id, url FROM items
		WHERE present = true AND image_url IS NULL
		ORDER BY COALESCE(published_at,'epoch'::timestamptz) DESC
		LIMIT $1`, limit)
	if err != nil {
		fmt.Printf("og query: %v\n", err)
		return 0, 0
	}
	type target struct{ id, url string }
	var targets []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.url); err != nil {
			rows.Close()
			fmt.Printf("og scan: %v\n", err)
			return 0, 0
		}
		targets = append(targets, t)
	}
	rows.Close()

	resolver := ingest.NewPageResolver()
	imgN, vidN := 0, 0
	for _, t := range targets {
		img, vid, _ := resolver.Resolve(ctx, t.url)
		if img != nil {
			tag, err := pool.Exec(ctx,
				`UPDATE items SET image_url=$1, updated_at=now() WHERE id=$2 AND image_url IS NULL`,
				*img, t.id)
			if err == nil {
				imgN += int(tag.RowsAffected())
			}
		}
		if vid != nil {
			tag, err := pool.Exec(ctx,
				`UPDATE items SET video_url=$1, updated_at=now() WHERE id=$2 AND video_url IS NULL`,
				*vid, t.id)
			if err == nil {
				vidN += int(tag.RowsAffected())
			}
		}
	}
	return imgN, vidN
}
