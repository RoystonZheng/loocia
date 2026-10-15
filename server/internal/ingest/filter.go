package ingest

import (
	"context"
	"strings"
)

// KeywordFilterSource applies an optional topic filter before items enter the
// raw queue. Empty include keywords mean "include everything"; exclusions win.
// It is deliberately a small deterministic filter so a source plan remains
// usable without an LLM or a domain-specific plugin.
type KeywordFilterSource struct {
	inner   Source
	include []string
	exclude []string
}

func NewKeywordFilterSource(inner Source, include, exclude []string) Source {
	return &KeywordFilterSource{inner: inner, include: normalizeKeywords(include), exclude: normalizeKeywords(exclude)}
}

func (s *KeywordFilterSource) Name() string { return s.inner.Name() }

func (s *KeywordFilterSource) Fetch(ctx context.Context) ([]RawItem, error) {
	items, err := s.inner.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	if len(s.include) == 0 && len(s.exclude) == 0 {
		return items, nil
	}
	out := make([]RawItem, 0, len(items))
	for _, item := range items {
		haystack := strings.ToLower(item.Title)
		if item.RawContent != nil {
			haystack += "\n" + strings.ToLower(*item.RawContent)
		}
		if matchesAny(haystack, s.exclude) || (len(s.include) > 0 && !matchesAny(haystack, s.include)) {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func normalizeKeywords(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func matchesAny(haystack string, values []string) bool {
	for _, value := range values {
		if strings.Contains(haystack, value) {
			return true
		}
	}
	return false
}
