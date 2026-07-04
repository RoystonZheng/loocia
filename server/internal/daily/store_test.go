package daily

import (
	"context"
	"errors"
	"testing"
)

func TestUpsertThenGetRoundTrips(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	in := sampleReport("2026-05-07")
	if err := s.Upsert(ctx, in); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := s.GetByDate(ctx, "2026-05-07")
	if err != nil {
		t.Fatalf("GetByDate: %v", err)
	}
	if got.Lead == nil || got.Lead.Title != "今日导语" {
		t.Fatalf("lead: %+v", got.Lead)
	}
	if len(got.Sections) != 1 || got.Sections[0].Label != "模型发布/更新" || len(got.Sections[0].Items) != 1 {
		t.Fatalf("sections: %+v", got.Sections)
	}
	if got.Sections[0].Items[0].Permalink == nil || *got.Sections[0].Items[0].Permalink != "/items/x1" {
		t.Fatalf("permalink: %+v", got.Sections[0].Items[0])
	}
	if len(got.Flashes) != 1 || got.Flashes[0].PublishedAt == nil {
		t.Fatalf("flashes: %+v", got.Flashes)
	}
	if !got.WindowStart.Equal(in.WindowStart) || !got.WindowEnd.Equal(in.WindowEnd) {
		t.Fatalf("window: %v %v", got.WindowStart, got.WindowEnd)
	}
}

func TestUpsertReplacesSameDate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.Upsert(ctx, sampleReport("2026-05-07")); err != nil {
		t.Fatal(err)
	}
	updated := sampleReport("2026-05-07")
	updated.Lead = nil // regeneration without lead
	updated.Sections = nil
	if err := s.Upsert(ctx, updated); err != nil {
		t.Fatalf("second Upsert: %v", err)
	}
	got, err := s.GetByDate(ctx, "2026-05-07")
	if err != nil {
		t.Fatal(err)
	}
	if got.Lead != nil {
		t.Fatalf("lead should be nil after replace: %+v", got.Lead)
	}
	if len(got.Sections) != 0 {
		t.Fatalf("sections should be empty after replace: %+v", got.Sections)
	}
}

func TestGetLatestAndListRecent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, d := range []string{"2026-05-05", "2026-05-07", "2026-05-06"} {
		if err := s.Upsert(ctx, sampleReport(d)); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := s.GetLatest(ctx)
	if err != nil {
		t.Fatalf("GetLatest: %v", err)
	}
	if latest.Date != "2026-05-07" {
		t.Fatalf("latest: %s", latest.Date)
	}
	entries, err := s.ListRecent(ctx, 2)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(entries) != 2 || entries[0].Date != "2026-05-07" || entries[1].Date != "2026-05-06" {
		t.Fatalf("entries: %+v", entries)
	}
	if entries[0].LeadTitle == nil || *entries[0].LeadTitle != "今日导语" {
		t.Fatalf("entry leadTitle: %+v", entries[0])
	}
}

func TestNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetLatest(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetLatest empty: %v", err)
	}
	if _, err := s.GetByDate(context.Background(), "2026-01-01"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByDate missing: %v", err)
	}
}
