package items

import (
	"context"
	"testing"
)

func TestEnsureSchemaIsIdempotentAndCreatesTable(t *testing.T) {
	s := newTestStore(t) // already calls EnsureSchema once
	// Running again must not error.
	if err := s.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("second EnsureSchema: %v", err)
	}
	var regclass *string
	err := s.pool.QueryRow(context.Background(), "SELECT to_regclass('public.items')::text").Scan(&regclass)
	if err != nil {
		t.Fatalf("to_regclass query: %v", err)
	}
	if regclass == nil || *regclass != "items" {
		t.Fatalf("items table not found, got %v", regclass)
	}
	// pg_trgm extension present.
	var ext int
	if err := s.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM pg_extension WHERE extname='pg_trgm'").Scan(&ext); err != nil {
		t.Fatalf("pg_extension query: %v", err)
	}
	if ext != 1 {
		t.Fatalf("pg_trgm extension not installed")
	}
}
