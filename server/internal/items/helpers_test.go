package items

import (
	"context"
	"os"
	"testing"
	"time"

	"aihot-server/internal/db"
)

func newTestStore(t *testing.T) *Store {
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
	s := New(pool)
	if err := s.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	if _, err := pool.Exec(context.Background(), "TRUNCATE items CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return s
}

func strptr(s string) *string { return &s }
func intptr(i int) *int       { return &i }

// sampleItem returns a minimal valid item with the given id and published time.
func sampleItem(id string, published time.Time) Item {
	return Item{
		ID:          id,
		Title:       "title-" + id,
		URL:         "https://example.com/" + id,
		Permalink:   "/items/" + id,
		Source:      "Example",
		PublishedAt: &published,
		Present:     true,
	}
}
