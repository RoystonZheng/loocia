// hotpass runs one clustering + heat pass over the trailing window.
//
//	AIHOT_DATABASE_URL=postgres://... AIHOT_LLM_API_KEY=... \
//	  go run ./cmd/hotpass [-window-hours 72]
//
// The LLM key is REQUIRED — grouping is the LLM's job.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"aihot-server/internal/cluster"
	"aihot-server/internal/db"
	"aihot-server/internal/items"
	"aihot-server/internal/llm"
)

func main() {
	windowHours := flag.Int("window-hours", 72, "trailing window size in hours")
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

	cs := cluster.NewStore(pool)
	if err := cs.EnsureSchema(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "schema:", err)
		os.Exit(1)
	}

	p := cluster.NewPass(items.New(pool), cs, client)
	res, err := p.Run(ctx, time.Duration(*windowHours)*time.Hour, time.Now().UTC())
	if err != nil {
		fmt.Fprintln(os.Stderr, "run:", err)
		os.Exit(1)
	}
	fmt.Printf("hotpass: %d window items → %d clusters\n", res.WindowItems, res.Clusters)
}
