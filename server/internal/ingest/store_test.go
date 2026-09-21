package ingest

import (
	"context"
	"testing"
	"time"
)

func TestInsertRawIsIdempotent(t *testing.T) {
	s := newTestRawStore(t)
	ctx := context.Background()
	r := sampleRaw("https://ex.com/a", time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))

	inserted, err := s.InsertRaw(ctx, r)
	if err != nil {
		t.Fatalf("first InsertRaw: %v", err)
	}
	if !inserted {
		t.Fatal("first insert should report inserted=true")
	}
	inserted, err = s.InsertRaw(ctx, r)
	if err != nil {
		t.Fatalf("second InsertRaw: %v", err)
	}
	if inserted {
		t.Fatal("second insert of same id should report inserted=false")
	}

	var count int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM raw_items").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("want 1 row, got %d", count)
	}
}

func TestListUnprocessedAndMarkProcessed(t *testing.T) {
	s := newTestRawStore(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	if _, err := s.InsertRaw(ctx, sampleRaw("https://ex.com/a", base)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertRaw(ctx, sampleRaw("https://ex.com/b", base.Add(time.Hour))); err != nil {
		t.Fatal(err)
	}

	un, err := s.ListUnprocessed(ctx, 10)
	if err != nil {
		t.Fatalf("ListUnprocessed: %v", err)
	}
	if len(un) != 2 {
		t.Fatalf("want 2 unprocessed, got %d", len(un))
	}

	if err := s.MarkProcessed(ctx, un[0].ID); err != nil {
		t.Fatalf("MarkProcessed: %v", err)
	}
	un2, err := s.ListUnprocessed(ctx, 10)
	if err != nil {
		t.Fatalf("ListUnprocessed 2: %v", err)
	}
	if len(un2) != 1 {
		t.Fatalf("want 1 unprocessed after mark, got %d", len(un2))
	}
	if un2[0].ID == un[0].ID {
		t.Fatal("processed item still returned as unprocessed")
	}
}

func TestListUnprocessedPrioritizesAIHOT(t *testing.T) {
	s := newTestRawStore(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	rss := sampleRaw("https://ex.com/old-rss", base)
	if _, err := s.InsertRaw(ctx, rss); err != nil {
		t.Fatal(err)
	}
	aihot := sampleRaw("https://ex.com/new-aihot", base.Add(time.Hour))
	aihot.SourceKind = SourceKindAIHOT
	if _, err := s.InsertRaw(ctx, aihot); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE raw_items
		SET fetched_at = CASE id WHEN $1 THEN $3::timestamptz ELSE $4::timestamptz END
		WHERE id IN ($1, $2)`, rss.ID, aihot.ID, base, base.Add(time.Hour)); err != nil {
		t.Fatalf("set fetched_at: %v", err)
	}

	un, err := s.ListUnprocessed(ctx, 10)
	if err != nil {
		t.Fatalf("ListUnprocessed: %v", err)
	}
	if len(un) != 2 {
		t.Fatalf("want 2 unprocessed, got %d", len(un))
	}
	if un[0].ID != aihot.ID {
		t.Fatalf("AIHOT should be processed before older RSS backlog: got first=%s want=%s", un[0].ID, aihot.ID)
	}
}

func TestInsertRawPersistsSourceRole(t *testing.T) {
	s := newTestRawStore(t)
	ctx := context.Background()
	r := sampleRaw("https://ex.com/official", time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))
	r.SourceRole = SourceRoleOfficial

	if _, err := s.InsertRaw(ctx, r); err != nil {
		t.Fatalf("InsertRaw: %v", err)
	}
	un, err := s.ListUnprocessed(ctx, 10)
	if err != nil {
		t.Fatalf("ListUnprocessed: %v", err)
	}
	if len(un) != 1 || un[0].SourceRole != SourceRoleOfficial {
		t.Fatalf("source role not persisted: %+v", un)
	}
}

func TestInsertRawDefaultsEmptySourceRole(t *testing.T) {
	s := newTestRawStore(t)
	ctx := context.Background()
	r := sampleRaw("https://ex.com/default-role", time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))
	r.SourceRole = ""

	if _, err := s.InsertRaw(ctx, r); err != nil {
		t.Fatalf("InsertRaw: %v", err)
	}
	un, err := s.ListUnprocessed(ctx, 10)
	if err != nil {
		t.Fatalf("ListUnprocessed: %v", err)
	}
	if len(un) != 1 || un[0].SourceRole != SourceRoleDiscovery {
		t.Fatalf("empty source role should default to discovery: %+v", un)
	}
}
