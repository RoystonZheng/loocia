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
var defaultSources = []SourceConfig{
	{Name: "OpenAI Blog", URL: "https://openai.com/news/rss.xml"},
	{Name: "Google AI Blog", URL: "https://blog.google/technology/ai/rss/"},
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
	return out, nil
}
