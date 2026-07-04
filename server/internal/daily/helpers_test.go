package daily

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
	s := NewStore(pool)
	if err := s.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	if _, err := pool.Exec(context.Background(), "TRUNCATE dailies"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return s
}

func sampleReport(date string) Report {
	gen := time.Date(2026, 5, 8, 1, 0, 0, 0, time.UTC)
	ws := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)
	perma := "/items/x1"
	pub := ws.Add(2 * time.Hour)
	return Report{
		Date: date, GeneratedAt: gen, WindowStart: ws, WindowEnd: ws.Add(24 * time.Hour),
		Lead: &Lead{Title: "今日导语", LeadParagraph: "这是导语段落。"},
		Sections: []Section{{
			Label: "模型发布/更新",
			Items: []SectionItem{{Title: "条目一", Summary: "摘要一", SourceURL: "https://x/1", SourceName: "Src", Permalink: &perma}},
		}},
		Flashes: []Flash{{Title: "快讯一", SourceName: "Src", SourceURL: "https://x/1", PublishedAt: &pub, Permalink: &perma}},
	}
}
