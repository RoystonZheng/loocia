package publicapi

import (
	"net/url"
	"testing"
	"time"

	"aihot-server/internal/items"
)

func vals(q string) url.Values {
	v, _ := url.ParseQuery(q)
	return v
}

var refNow = time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)

func TestParseDefaults(t *testing.T) {
	p, err := parseListParams(vals(""), refNow)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.Selected == nil || !*p.Selected {
		t.Fatalf("default mode should be selected: %+v", p.Selected)
	}
	if p.Limit != 50 {
		t.Fatalf("default take: %d", p.Limit)
	}
	if p.Since == nil || !p.Since.Equal(refNow.Add(-7*24*time.Hour)) {
		t.Fatalf("default since: %v", p.Since)
	}
}

func TestParseModeAll(t *testing.T) {
	p, _ := parseListParams(vals("mode=all"), refNow)
	if p.Selected != nil {
		t.Fatalf("mode=all should not constrain selected: %+v", p.Selected)
	}
}

func TestParseCategoryValidAndInvalid(t *testing.T) {
	p, err := parseListParams(vals("category=paper"), refNow)
	if err != nil || p.Category == nil || *p.Category != "paper" {
		t.Fatalf("valid category: %+v err=%v", p.Category, err)
	}
	if _, err := parseListParams(vals("category=nope"), refNow); err == nil {
		t.Fatal("invalid category should 400")
	}
}

func TestParseScoreMinValidAndInvalid(t *testing.T) {
	p, err := parseListParams(vals("mode=all&score_min=4"), refNow)
	if err != nil || p.ScoreMin == nil || *p.ScoreMin != 4 {
		t.Fatalf("valid score_min: %+v err=%v", p.ScoreMin, err)
	}
	for _, value := range []string{"0", "6", "abc"} {
		if _, err := parseListParams(vals("score_min="+value), refNow); err == nil {
			t.Fatalf("score_min=%s should 400", value)
		}
	}
}

func TestParseTakeBounds(t *testing.T) {
	if _, err := parseListParams(vals("take=0"), refNow); err == nil {
		t.Fatal("take=0 should 400")
	}
	if _, err := parseListParams(vals("take=101"), refNow); err == nil {
		t.Fatal("take=101 should 400")
	}
	if _, err := parseListParams(vals("take=abc"), refNow); err == nil {
		t.Fatal("take=abc should 400")
	}
	p, _ := parseListParams(vals("take=25"), refNow)
	if p.Limit != 25 {
		t.Fatalf("take=25: %d", p.Limit)
	}
}

func TestParseSinceClampAndFuture(t *testing.T) {
	within := refNow.Add(-2 * 24 * time.Hour).Format(time.RFC3339)
	p, err := parseListParams(vals("since="+url.QueryEscape(within)), refNow)
	if err != nil || p.Since == nil || !p.Since.Equal(refNow.Add(-2*24*time.Hour)) {
		t.Fatalf("within-7d since: %v err=%v", p.Since, err)
	}
	old := refNow.Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	p, _ = parseListParams(vals("since="+url.QueryEscape(old)), refNow)
	if !p.Since.Equal(refNow.Add(-7 * 24 * time.Hour)) {
		t.Fatalf("old since should clamp to now-7d: %v", p.Since)
	}
	future := refNow.Add(10 * time.Minute).Format(time.RFC3339)
	if _, err := parseListParams(vals("since="+url.QueryEscape(future)), refNow); err == nil {
		t.Fatal("future since should 400")
	}
	if _, err := parseListParams(vals("since=not-a-date"), refNow); err == nil {
		t.Fatal("malformed since should 400")
	}
}

func TestParseQLenRules(t *testing.T) {
	p, _ := parseListParams(vals("q=a"), refNow)
	if p.Q != nil {
		t.Fatalf("1-char q should be ignored: %v", p.Q)
	}
	p, _ = parseListParams(vals("q=OpenAI"), refNow)
	if p.Q == nil || *p.Q != "OpenAI" {
		t.Fatalf("q: %v", p.Q)
	}
	long := make([]byte, 250)
	for i := range long {
		long[i] = 'x'
	}
	p, _ = parseListParams(vals("q="+string(long)), refNow)
	if p.Q == nil || len(*p.Q) != 200 {
		t.Fatalf("q should truncate to 200, got %d", lenPtr(p.Q))
	}
}

func TestParseSearchIsAllTime(t *testing.T) {
	// A text search drops the default 7-day floor so the whole archive is
	// searchable; browsing (no q) keeps the window.
	p, _ := parseListParams(vals("q=Harness"), refNow)
	if p.Q == nil || *p.Q != "Harness" {
		t.Fatalf("q: %v", p.Q)
	}
	if p.Since != nil {
		t.Fatalf("search should have no since floor, got %v", p.Since)
	}
	// An explicit older since with a search is honored, not clamped to now-7d.
	old := refNow.Add(-90 * 24 * time.Hour).UTC().Format(time.RFC3339)
	p, _ = parseListParams(vals("q=Harness&since="+old), refNow)
	if p.Since == nil || !p.Since.Equal(refNow.Add(-90*24*time.Hour)) {
		t.Fatalf("search since should not clamp: %v", p.Since)
	}
	// Browsing (no q) still defaults to the 7-day window.
	p, _ = parseListParams(vals(""), refNow)
	if p.Since == nil || !p.Since.Equal(refNow.Add(-7*24*time.Hour)) {
		t.Fatalf("browse should keep 7d window: %v", p.Since)
	}
}

func TestParseSourceKind(t *testing.T) {
	// mp is a historical archive → browsing it drops the 7-day floor.
	p, err := parseListParams(vals("source_kind=mp"), refNow)
	if err != nil {
		t.Fatalf("source_kind=mp: %v", err)
	}
	if p.SourceKind == nil || *p.SourceKind != "mp" {
		t.Fatalf("SourceKind: %v", p.SourceKind)
	}
	if p.Since != nil {
		t.Fatalf("mp browse should have no since floor, got %v", p.Since)
	}
	// rss keeps the 7-day freshness window.
	p, err = parseListParams(vals("source_kind=rss"), refNow)
	if err != nil {
		t.Fatalf("source_kind=rss: %v", err)
	}
	if p.SourceKind == nil || *p.SourceKind != "rss" {
		t.Fatalf("SourceKind: %v", p.SourceKind)
	}
	if p.Since == nil || !p.Since.Equal(refNow.Add(-7*24*time.Hour)) {
		t.Fatalf("rss should keep 7d window: %v", p.Since)
	}
	for _, kind := range []string{"html", "aihot"} {
		p, err = parseListParams(vals("source_kind="+kind), refNow)
		if err != nil {
			t.Fatalf("source_kind=%s: %v", kind, err)
		}
		if p.SourceKind == nil || *p.SourceKind != kind {
			t.Fatalf("SourceKind for %s: %v", kind, p.SourceKind)
		}
		if p.Since == nil || !p.Since.Equal(refNow.Add(-7*24*time.Hour)) {
			t.Fatalf("%s should keep 7d window: %v", kind, p.Since)
		}
	}
	// Invalid value → 400.
	if _, err := parseListParams(vals("source_kind=email"), refNow); err == nil {
		t.Fatal("invalid source_kind should 400")
	}
}

func lenPtr(s *string) int {
	if s == nil {
		return -1
	}
	return len(*s)
}

func TestParseCursorInvalidIgnored(t *testing.T) {
	p, err := parseListParams(vals("cursor=garbage!!!"), refNow)
	if err != nil {
		t.Fatalf("bad cursor must not error: %v", err)
	}
	if p.After != nil {
		t.Fatalf("bad cursor should yield nil After: %+v", p.After)
	}
	tok := encodeCursor(&items.Cursor{SortKey: refNow, ID: "x"})
	p, _ = parseListParams(vals("cursor="+url.QueryEscape(tok)), refNow)
	if p.After == nil || p.After.ID != "x" {
		t.Fatalf("good cursor should populate After: %+v", p.After)
	}
}
