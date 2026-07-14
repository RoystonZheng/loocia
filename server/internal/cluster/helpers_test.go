package cluster

import (
	"context"
	"os"
	"testing"
	"time"

	"aihot-server/internal/db"
	"aihot-server/internal/items"

	"github.com/jackc/pgx/v5/pgxpool"
)

// newLiveStores gives a cluster store + items store on a shared pool with
// clean clusters/items tables. Skips without the DSN.
func newLiveStores(t *testing.T) (*Store, *items.Store, *pgxpool.Pool) {
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
	cs := NewStore(pool)
	if err := cs.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("cluster EnsureSchema: %v", err)
	}
	is := items.New(pool)
	if err := is.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("items EnsureSchema: %v", err)
	}
	// CASCADE: items is referenced by item_terms (terms package FK).
	for _, tbl := range []string{"clusters", "items"} {
		if _, err := pool.Exec(context.Background(), "TRUNCATE "+tbl+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", tbl, err)
		}
	}
	return cs, is, pool
}

func seedItem(t *testing.T, is *items.Store, id, source string, score int, pub time.Time, selected bool) items.Item {
	t.Helper()
	summary := "摘要-" + id
	cat := items.CategoryAIModels
	sc := score
	it := items.Item{
		ID: id, Title: "标题-" + id, URL: "https://x/" + id, Permalink: "/items/" + id,
		Source: source, PublishedAt: &pub, Summary: &summary, Category: &cat, Score: &sc,
		Selected: selected, Present: true,
	}
	if err := is.Upsert(context.Background(), it); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
	return it
}
