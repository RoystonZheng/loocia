package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// RawItem is a fetched-but-unprocessed entry. P1.3 enriches it into items.Item.
type RawItem struct {
	ID          string // = RawID(URL); stable, equals the eventual items.id
	Source      string // human source name, e.g. "OpenAI Blog"
	SourceKind  string // "rss" | "mp_hot" | "x" | ...
	URL         string // original third-party URL
	Title       string // original (pre-translation) title
	PublishedAt *time.Time
	RawContent  *string // raw description/body from the feed, if any
}

// RawID derives a stable id from the canonical URL.
func RawID(url string) string {
	sum := sha256.Sum256([]byte(url))
	return hex.EncodeToString(sum[:])
}

// Source is anything that yields raw items (RSS feed, 公众号 export, X, ...).
type Source interface {
	Name() string
	Fetch(ctx context.Context) ([]RawItem, error)
}
