package pulse

import (
	"encoding/json"
	"fmt"
	"os"

	"aihot-server/internal/ingest"
)

// SourceConfig is one feed entry in the optional sources JSON file.
type SourceConfig struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// defaultSources are feeds verified reachable from the deploy host (Melos).
// Volume-heavy feeds (e.g. arXiv) are deliberately not defaults — each new item
// costs one LLM enrichment call. Override with a JSON file when needed.
// Chosen 2026-07-06 by probing reachability + freshness from the Melos host:
// each is reachable there and ships real article content in its RSS (not just a
// link stub), so LLM enrichment has substance to work with. Google AI Blog was
// dropped (unreachable from Melos, http=000) and Hacker News skipped (link-only
// descriptions → thin summaries). OpenAI publishes sporadically but stays.
var defaultSources = []SourceConfig{
	{Name: "OpenAI Blog", URL: "https://openai.com/news/rss.xml"},
	{Name: "TechCrunch AI", URL: "https://techcrunch.com/category/artificial-intelligence/feed/"},
	{Name: "Simon Willison", URL: "https://simonwillison.net/atom/everything/"},
	{Name: "MIT Technology Review AI", URL: "https://www.technologyreview.com/topic/artificial-intelligence/feed"},
}

// LoadSources returns the RSS sources: the embedded defaults when path is
// empty, else the JSON file at path ([{"name":...,"url":...}]).
func LoadSources(path string) ([]ingest.Source, error) {
	configs := defaultSources
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read sources file: %w", err)
		}
		configs = nil
		if err := json.Unmarshal(data, &configs); err != nil {
			return nil, fmt.Errorf("parse sources file: %w", err)
		}
	}
	var out []ingest.Source
	for i, c := range configs {
		if c.Name == "" || c.URL == "" {
			return nil, fmt.Errorf("source %d: name and url are required", i)
		}
		out = append(out, ingest.NewRSSSource(c.Name, c.URL))
	}
	// Append the WeChat 公众号 corpus source when configured and the dir exists.
	// Absent/missing dir → RSS-only (unchanged behavior).
	if dir := os.Getenv("AIHOT_MP_CORPUS_DIR"); dir != "" {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			out = append(out, ingest.NewMPCorpusSource(dir, "WeChat MP"))
		}
	}
	return out, nil
}
