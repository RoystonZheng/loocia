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
	SourceKind  string // "rss" | "html" | "mp" | "aihot"
	SourceRole  string // "official" | "professional" | "discovery"
	URL         string // original third-party URL
	Title       string // original (pre-translation) title
	PublishedAt *time.Time
	RawContent  *string // raw description/body from the feed, if any
	ImageURL    *string // representative image URL from the feed, if any
	VideoURL    *string // representative video URL from the feed, if any
}

const (
	SourceKindRSS   = "rss"
	SourceKindHTML  = "html"
	SourceKindMP    = "mp"
	SourceKindAIHOT = "aihot"

	SourceRoleOfficial     = "official"
	SourceRoleProfessional = "professional"
	SourceRoleDiscovery    = "discovery"
)

func DefaultSourceRole(role string) string {
	if role == "" {
		return SourceRoleDiscovery
	}
	return role
}

func ValidSourceRole(role string) bool {
	switch role {
	case SourceRoleOfficial, SourceRoleProfessional, SourceRoleDiscovery:
		return true
	default:
		return false
	}
}

func ValidSourceKind(kind string) bool {
	switch kind {
	case SourceKindRSS, SourceKindHTML, SourceKindMP, SourceKindAIHOT:
		return true
	default:
		return false
	}
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
