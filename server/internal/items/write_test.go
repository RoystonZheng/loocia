package items

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestUpsertThenGetByIDRoundTrips(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pub := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

	in := sampleItem("a1", pub)
	in.TitleEN = strptr("English Title")
	in.Summary = strptr("摘要")
	in.Category = strptr(CategoryAIModels)
	in.Score = intptr(88)
	in.Selected = true

	if err := s.Upsert(ctx, in); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := s.GetByID(ctx, "a1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Title != in.Title || got.TitleEN == nil || *got.TitleEN != "English Title" {
		t.Fatalf("title round-trip mismatch: %+v", got)
	}
	if got.Category == nil || *got.Category != CategoryAIModels || got.Score == nil || *got.Score != 88 {
		t.Fatalf("category/score round-trip mismatch: %+v", got)
	}
	if !got.Selected || !got.Present {
		t.Fatalf("bool round-trip mismatch: %+v", got)
	}
	if got.PublishedAt == nil || !got.PublishedAt.Equal(pub) {
		t.Fatalf("published_at round-trip mismatch: %v", got.PublishedAt)
	}
}

func TestUpsertUpdatesExistingRow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pub := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

	if err := s.Upsert(ctx, sampleItem("a1", pub)); err != nil {
		t.Fatalf("first Upsert: %v", err)
	}
	updated := sampleItem("a1", pub)
	updated.Title = "new title"
	updated.Summary = strptr("new summary")
	if err := s.Upsert(ctx, updated); err != nil {
		t.Fatalf("second Upsert: %v", err)
	}
	got, err := s.GetByID(ctx, "a1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Title != "new title" || got.Summary == nil || *got.Summary != "new summary" {
		t.Fatalf("update not applied: %+v", got)
	}

	var count int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM items WHERE id='a1'").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("want 1 row, got %d (upsert duplicated)", count)
	}
}

func TestGetByIDNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetByID(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
