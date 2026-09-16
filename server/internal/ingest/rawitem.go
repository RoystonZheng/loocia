package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/url"
	"path"
	"strings"
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
func RawID(rawURL string) string {
	sum := sha256.Sum256([]byte(CanonicalURL(rawURL)))
	return hex.EncodeToString(sum[:])
}

// CanonicalURL normalizes web URLs for cross-source dedupe while leaving
// stable non-URL seeds such as "aihot:<id>" unchanged.
func CanonicalURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return rawURL
	}

	scheme := strings.ToLower(parsed.Scheme)
	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	parsed.Scheme = scheme
	if port != "" && !isDefaultPort(scheme, port) {
		parsed.Host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		parsed.Host = "[" + host + "]"
	} else {
		parsed.Host = host
	}

	parsed.Fragment = ""
	parsed.RawFragment = ""
	parsed.RawPath = ""
	parsed.Path = canonicalPath(parsed.Path)

	query := parsed.Query()
	for key := range query {
		if isTrackingParam(key) {
			delete(query, key)
		}
	}
	parsed.RawQuery = query.Encode()

	return parsed.String()
}

func canonicalPath(value string) string {
	if value == "" || value == "/" {
		return ""
	}
	hadTrailingSlash := strings.HasSuffix(value, "/")
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == "/" {
		return ""
	}
	if hadTrailingSlash && !strings.HasSuffix(cleaned, "/") {
		cleaned += "/"
	}
	return cleaned
}

func isDefaultPort(scheme, port string) bool {
	return (scheme == "http" && port == "80") || (scheme == "https" && port == "443")
}

func isTrackingParam(key string) bool {
	switch strings.ToLower(key) {
	case "utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content", "utm_id",
		"fbclid", "gclid", "dclid", "yclid", "mc_cid", "mc_eid", "_hsenc", "_hsmi", "igshid":
		return true
	default:
		return false
	}
}

// Source is anything that yields raw items (RSS feed, 公众号 export, X, ...).
type Source interface {
	Name() string
	Fetch(ctx context.Context) ([]RawItem, error)
}
