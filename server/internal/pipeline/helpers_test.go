package pipeline

import (
	"context"
	"os"
	"testing"

	"aihot-server/internal/db"
	"aihot-server/internal/ingest"
	"aihot-server/internal/items"

	"github.com/jackc/pgx/v5/pgxpool"
)

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
	return pool
}

func testStores(t *testing.T, pool *pgxpool.Pool) (*ingest.RawStore, *items.Store) {
	t.Helper()
	ctx := context.Background()
	raw := ingest.NewRawStore(pool)
	if err := raw.EnsureSchema(ctx); err != nil {
		t.Fatalf("raw EnsureSchema: %v", err)
	}
	it := items.New(pool)
	if err := it.EnsureSchema(ctx); err != nil {
		t.Fatalf("items EnsureSchema: %v", err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE raw_items"); err != nil {
		t.Fatalf("truncate raw_items: %v", err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE items CASCADE"); err != nil {
		t.Fatalf("truncate items: %v", err)
	}
	return raw, it
}

type fakeEnricher struct {
	out Enrichment
	err error
}

func (f fakeEnricher) Enrich(ctx context.Context, r ingest.RawItem) (Enrichment, error) {
	return f.out, f.err
}
