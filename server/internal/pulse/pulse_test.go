package pulse

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"aihot-server/internal/db"
	"aihot-server/internal/ingest"

	"github.com/jackc/pgx/v5/pgxpool"
)

func pulseRSS(now time.Time) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>T</title>
  <item><title>Pulse Item One</title><link>https://ex.com/pulse-1</link>
    <description>body one</description><pubDate>%s</pubDate></item>
  <item><title>Pulse Item Two</title><link>https://ex.com/pulse-2</link>
    <description>body two</description><pubDate>%s</pubDate></item>
</channel></rss>`, now.Add(-2*time.Hour).UTC().Format(time.RFC1123), now.Add(-time.Hour).UTC().Format(time.RFC1123))
}

type fakeLLM struct{ reply string }

func (f fakeLLM) Complete(ctx context.Context, system, user string) (string, error) {
	return f.reply, nil
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("AIHOT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AIHOT_TEST_DATABASE_URL not set")
	}
	pool, err := db.NewPool(context.Background(), dsn)
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	t.Cleanup(pool.Close)
	for _, tbl := range []string{"raw_items", "items"} {
		// Tables may not exist on a fresh DB; Run ensures schemas, so ignore errors
		// here. CASCADE: items is referenced by item_terms (terms package FK).
		_, _ = pool.Exec(context.Background(), "TRUNCATE "+tbl+" CASCADE")
	}
	return pool
}

func TestRunIngestsAndEnriches(t *testing.T) {
	pool := testPool(t)
	rss := pulseRSS(time.Now().UTC())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(rss))
	}))
	defer srv.Close()

	sum, err := Run(context.Background(), Deps{
		Pool:    pool,
		LLM:     fakeLLM{reply: `{"title_cn":"中文标题","summary_cn":"摘要","category":"ai-models","relevance":90,"score":80,"selected":true}`},
		Sources: []ingest.Source{ingest.NewRSSSource("Test Feed", srv.URL)},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sum.Inserted != 2 || sum.Processed != 2 || sum.Failed != 0 {
		t.Fatalf("summary: %+v", sum)
	}

	// Items landed enriched.
	var count int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM items WHERE title='中文标题'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("enriched items: %d", count)
	}

	// Second run: idempotent — nothing new inserted or processed.
	sum2, err := Run(context.Background(), Deps{
		Pool:    pool,
		LLM:     fakeLLM{reply: `{}`},
		Sources: []ingest.Source{ingest.NewRSSSource("Test Feed", srv.URL)},
	})
	if err != nil {
		t.Fatalf("Run 2: %v", err)
	}
	if sum2.Inserted != 0 || sum2.Processed != 0 {
		t.Fatalf("second run should be a no-op: %+v", sum2)
	}
}

func TestRunStopsWhenNoProgress(t *testing.T) {
	pool := testPool(t)
	rss := pulseRSS(time.Now().UTC())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(rss))
	}))
	defer srv.Close()

	// LLM returns garbage → every enrichment fails → items stay unprocessed.
	// The drain loop must stop after the first no-progress batch, not spin to the cap.
	sum, err := Run(context.Background(), Deps{
		Pool:    pool,
		LLM:     fakeLLM{reply: `garbage not json`},
		Sources: []ingest.Source{ingest.NewRSSSource("Test Feed", srv.URL)},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sum.Processed != 0 || sum.Failed != 2 || sum.Batches != 1 {
		t.Fatalf("no-progress summary: %+v", sum)
	}
}
