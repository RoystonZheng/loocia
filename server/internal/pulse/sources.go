package pulse

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"aihot-server/internal/ingest"
)

// SourceConfig is one feed entry in the optional sources JSON file.
type SourceConfig struct {
	ID                 string `json:"id,omitempty"`
	Name               string `json:"name"`
	URL                string `json:"url"`
	Adapter            string `json:"adapter,omitempty"`
	SourceKind         string `json:"source_kind,omitempty"`
	SourceRole         string `json:"source_role,omitempty"`
	Enabled            *bool  `json:"enabled,omitempty"`
	MaxItemsPerRun     int    `json:"max_items_per_run,omitempty"`
	FetchLimit         int    `json:"fetch_limit,omitempty"`
	MaxSecondaryPerRun int    `json:"max_secondary_per_run,omitempty"`
	SecondaryCap       int    `json:"secondary_cap,omitempty"`
	TimeoutSeconds     int    `json:"timeout_seconds,omitempty"`
	Window             string `json:"window,omitempty"`
	RateLimit          string `json:"rate_limit,omitempty"`
}

const (
	adapterRSS            = "rss"
	adapterMPCorpus       = "mp_corpus"
	adapterAnthropicHTML  = "anthropic_news_html"
	adapterAIHOTV1Items   = "aihot_v1_items"
	defaultAIHOTWindow    = "7d"
	defaultAIHOTLimit     = 50
	defaultAIHOTSecondary = 10
)

// defaultSources keep the original lightweight RSS defaults and add the first
// source-expansion batch. Volume-heavy feeds (e.g. arXiv) are deliberately not
// defaults because each new item costs one LLM enrichment call. Override with a
// JSON file when a deploy environment needs a narrower or wider source set.
// Reddit MachineLearning is retained as a known source but disabled by default:
// it returned 429 in local validation and should be enabled only after deploy
// host verification.
var defaultSources = []SourceConfig{
	{Name: "OpenAI Blog", URL: "https://openai.com/news/rss.xml", Adapter: adapterRSS, SourceKind: ingest.SourceKindRSS, SourceRole: ingest.SourceRoleOfficial},
	{Name: "TechCrunch AI", URL: "https://techcrunch.com/category/artificial-intelligence/feed/", Adapter: adapterRSS, SourceKind: ingest.SourceKindRSS, SourceRole: ingest.SourceRoleProfessional},
	{Name: "Simon Willison", URL: "https://simonwillison.net/atom/everything/", Adapter: adapterRSS, SourceKind: ingest.SourceKindRSS, SourceRole: ingest.SourceRoleProfessional},
	{Name: "MIT Technology Review AI", URL: "https://www.technologyreview.com/topic/artificial-intelligence/feed", Adapter: adapterRSS, SourceKind: ingest.SourceKindRSS, SourceRole: ingest.SourceRoleProfessional},
	{Name: "Anthropic News", URL: "https://www.anthropic.com/news", Adapter: adapterAnthropicHTML, SourceKind: ingest.SourceKindHTML, SourceRole: ingest.SourceRoleOfficial, MaxItemsPerRun: 30},
	{Name: "Google DeepMind", URL: "https://deepmind.google/blog/rss.xml", Adapter: adapterRSS, SourceKind: ingest.SourceKindRSS, SourceRole: ingest.SourceRoleOfficial},
	{Name: "Cloudflare AI", URL: "https://blog.cloudflare.com/tag/ai/rss/", Adapter: adapterRSS, SourceKind: ingest.SourceKindRSS, SourceRole: ingest.SourceRoleOfficial},
	{Name: "NVIDIA Generative AI", URL: "https://developer.nvidia.com/blog/category/generative-ai/feed/", Adapter: adapterRSS, SourceKind: ingest.SourceKindRSS, SourceRole: ingest.SourceRoleOfficial},
	{Name: "人人都是产品经理", URL: "https://www.woshipm.com/feed", Adapter: adapterRSS, SourceKind: ingest.SourceKindRSS, SourceRole: ingest.SourceRoleProfessional},
	{Name: "极客公园", URL: "https://www.geekpark.net/rss", Adapter: adapterRSS, SourceKind: ingest.SourceKindRSS, SourceRole: ingest.SourceRoleProfessional},
	{Name: "Reddit LocalLLaMA", URL: "https://www.reddit.com/r/LocalLLaMA/new/.rss", Adapter: adapterRSS, SourceKind: ingest.SourceKindRSS, SourceRole: ingest.SourceRoleDiscovery},
	{Name: "Reddit MachineLearning", URL: "https://www.reddit.com/r/MachineLearning/new/.rss", Adapter: adapterRSS, SourceKind: ingest.SourceKindRSS, SourceRole: ingest.SourceRoleDiscovery, Enabled: boolPtr(false)},
	{Name: "AIHOT", URL: "https://aihot.news/api/v1/items", Adapter: adapterAIHOTV1Items, SourceKind: ingest.SourceKindAIHOT, SourceRole: ingest.SourceRoleDiscovery, MaxItemsPerRun: defaultAIHOTLimit, MaxSecondaryPerRun: defaultAIHOTSecondary, Window: defaultAIHOTWindow},
}

// LoadSources returns configured ingest sources: embedded defaults when path is
// empty, else the JSON file at path. Old [{"name":...,"url":...}] files remain
// valid RSS configs.
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
		src, enabled, err := buildSource(c, i)
		if err != nil {
			return nil, err
		}
		if !enabled {
			continue
		}
		out = append(out, src)
	}
	// Append the WeChat 公众号 corpus source when configured and the dir exists.
	// Absent/missing dir → RSS-only (unchanged behavior).
	if dir := os.Getenv("AIHOT_MP_CORPUS_DIR"); dir != "" {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			out = append(out, ingest.NewMPCorpusSourceWithOptions(dir, "WeChat MP", ingest.MPCorpusSourceOptions{
				DefaultRole:      ingest.SourceRoleProfessional,
				OfficialAccounts: parseCSV(os.Getenv("AIHOT_MP_OFFICIAL_ACCOUNTS")),
			}))
		}
	}
	return out, nil
}

type normalizedSourceConfig struct {
	SourceConfig
	adapter      string
	sourceKind   string
	sourceRole   string
	enabled      bool
	maxItems     int
	maxSecondary int
	timeout      time.Duration
	window       string
}

func buildSource(c SourceConfig, index int) (ingest.Source, bool, error) {
	n, err := normalizeSourceConfig(c, index)
	if err != nil {
		return nil, false, err
	}
	if !n.enabled {
		return nil, false, nil
	}
	switch n.adapter {
	case adapterRSS:
		return ingest.NewRSSSourceWithOptions(n.Name, n.URL, ingest.RSSSourceOptions{
			SourceKind:     n.sourceKind,
			SourceRole:     n.sourceRole,
			MaxItemsPerRun: n.maxItems,
			Timeout:        n.timeout,
		}), true, nil
	case adapterAnthropicHTML:
		return ingest.NewAnthropicNewsSource(n.Name, n.URL, ingest.HTMLSourceOptions{
			SourceKind:     n.sourceKind,
			SourceRole:     n.sourceRole,
			MaxItemsPerRun: n.maxItems,
			Timeout:        n.timeout,
		}), true, nil
	case adapterAIHOTV1Items:
		return ingest.NewAIHOTSource(n.Name, n.URL, ingest.AIHOTSourceOptions{
			SourceRole:         n.sourceRole,
			MaxItemsPerRun:     n.maxItems,
			MaxSecondaryPerRun: n.maxSecondary,
			Window:             n.window,
			Timeout:            n.timeout,
		}), true, nil
	case adapterMPCorpus:
		return ingest.NewMPCorpusSourceWithOptions(n.URL, n.Name, ingest.MPCorpusSourceOptions{
			DefaultRole:      n.sourceRole,
			OfficialAccounts: parseCSV(os.Getenv("AIHOT_MP_OFFICIAL_ACCOUNTS")),
		}), true, nil
	default:
		return nil, false, fmt.Errorf("source %d: unknown adapter %q", index, n.adapter)
	}
}

func normalizeSourceConfig(c SourceConfig, index int) (normalizedSourceConfig, error) {
	enabled := c.Enabled == nil || *c.Enabled
	if !enabled {
		return normalizedSourceConfig{SourceConfig: c, enabled: false}, nil
	}
	c.Name = strings.TrimSpace(c.Name)
	c.URL = strings.TrimSpace(c.URL)
	if c.Name == "" || c.URL == "" {
		return normalizedSourceConfig{}, fmt.Errorf("source %d: name and url are required", index)
	}
	adapter := strings.TrimSpace(c.Adapter)
	if adapter == "" {
		adapter = adapterRSS
	}
	sourceKind := strings.TrimSpace(c.SourceKind)
	if sourceKind == "" {
		sourceKind = defaultKindForAdapter(adapter)
	}
	if sourceKind == "" || !ingest.ValidSourceKind(sourceKind) {
		return normalizedSourceConfig{}, fmt.Errorf("source %d: invalid source_kind %q", index, c.SourceKind)
	}
	if expected := defaultKindForAdapter(adapter); expected == "" {
		return normalizedSourceConfig{}, fmt.Errorf("source %d: unknown adapter %q", index, adapter)
	} else if sourceKind != expected {
		return normalizedSourceConfig{}, fmt.Errorf("source %d: adapter %q requires source_kind %q", index, adapter, expected)
	}
	sourceRole := strings.TrimSpace(c.SourceRole)
	if sourceRole == "" {
		sourceRole = ingest.SourceRoleDiscovery
		if adapter == adapterMPCorpus {
			sourceRole = ingest.SourceRoleProfessional
		}
	}
	if !ingest.ValidSourceRole(sourceRole) {
		return normalizedSourceConfig{}, fmt.Errorf("source %d: invalid source_role %q", index, c.SourceRole)
	}
	maxItems := c.MaxItemsPerRun
	if maxItems <= 0 {
		maxItems = c.FetchLimit
	}
	if adapter == adapterAIHOTV1Items && maxItems <= 0 {
		maxItems = defaultAIHOTLimit
	}
	maxSecondary := c.MaxSecondaryPerRun
	if maxSecondary <= 0 {
		maxSecondary = c.SecondaryCap
	}
	if adapter == adapterAIHOTV1Items && maxSecondary <= 0 {
		maxSecondary = defaultAIHOTSecondary
	}
	window := strings.TrimSpace(c.Window)
	if adapter == adapterAIHOTV1Items && window == "" {
		window = defaultAIHOTWindow
	}
	var timeout time.Duration
	if c.TimeoutSeconds > 0 {
		timeout = time.Duration(c.TimeoutSeconds) * time.Second
	}
	return normalizedSourceConfig{
		SourceConfig: c,
		adapter:      adapter,
		sourceKind:   sourceKind,
		sourceRole:   sourceRole,
		enabled:      true,
		maxItems:     maxItems,
		maxSecondary: maxSecondary,
		timeout:      timeout,
		window:       window,
	}, nil
}

func defaultKindForAdapter(adapter string) string {
	switch adapter {
	case adapterRSS:
		return ingest.SourceKindRSS
	case adapterAnthropicHTML:
		return ingest.SourceKindHTML
	case adapterAIHOTV1Items:
		return ingest.SourceKindAIHOT
	case adapterMPCorpus:
		return ingest.SourceKindMP
	default:
		return ""
	}
}

func parseCSV(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func boolPtr(v bool) *bool {
	return &v
}
