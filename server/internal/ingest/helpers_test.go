package ingest

import (
	"context"
	"os"
	"testing"
	"time"

	"aihot-server/internal/db"
)

func newTestRawStore(t *testing.T) *RawStore {
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
	s := NewRawStore(pool)
	if err := s.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	if _, err := pool.Exec(context.Background(), "TRUNCATE raw_items"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return s
}

func sampleRaw(url string, pub time.Time) RawItem {
	return RawItem{
		ID:          RawID(url),
		Source:      "Example",
		SourceKind:  "rss",
		URL:         url,
		Title:       "title for " + url,
		PublishedAt: &pub,
	}
}
