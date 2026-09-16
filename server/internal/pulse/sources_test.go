package pulse

import (
	"aihot-server/internal/ingest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadSourcesDefaults(t *testing.T) {
	srcs, err := LoadSources("")
	if err != nil {
		t.Fatalf("LoadSources(\"\"): %v", err)
	}
	if len(srcs) < 4 {
		t.Fatalf("want >=4 default sources, got %d", len(srcs))
	}
	names := map[string]bool{}
	for _, s := range srcs {
		names[s.Name()] = true
	}
	// Curated 2026-07-06: reachable-from-Melos AI feeds with real article content.
	for _, want := range []string{
		"OpenAI Blog",
		"TechCrunch AI",
		"Simon Willison",
		"MIT Technology Review AI",
		"Anthropic News",
		"Google DeepMind",
		"Cloudflare AI",
		"NVIDIA Generative AI",
		"人人都是产品经理",
		"极客公园",
		"Reddit LocalLLaMA",
		"AIHOT",
	} {
		if !names[want] {
			t.Fatalf("missing default source %q; got %v", want, names)
		}
	}
	if names["Reddit MachineLearning"] {
		t.Fatalf("Reddit MachineLearning should be disabled by default; got %v", names)
	}
	// Google AI Blog was dropped (unreachable from Melos).
	if names["Google AI Blog"] {
		t.Fatalf("Google AI Blog should have been removed; got %v", names)
	}
}

func TestDefaultSourceConfigRoles(t *testing.T) {
	roles := map[string]string{}
	enabled := map[string]bool{}
	for i, c := range defaultSources {
		n, err := normalizeSourceConfig(c, i)
		if err != nil {
			t.Fatalf("normalize default %q: %v", c.Name, err)
		}
		roles[c.Name] = n.sourceRole
		enabled[c.Name] = n.enabled
	}
	for name, want := range map[string]string{
		"OpenAI Blog":            "official",
		"Anthropic News":         "official",
		"Google DeepMind":        "official",
		"Cloudflare AI":          "official",
		"NVIDIA Generative AI":   "official",
		"TechCrunch AI":          "professional",
		"人人都是产品经理":               "professional",
		"极客公园":                   "professional",
		"Reddit LocalLLaMA":      "discovery",
		"Reddit MachineLearning": "",
		"AIHOT":                  "discovery",
	} {
		if name == "Reddit MachineLearning" {
			if enabled[name] {
				t.Fatal("Reddit MachineLearning should be disabled")
			}
			continue
		}
		if roles[name] != want {
			t.Fatalf("%s role: got %q want %q", name, roles[name], want)
		}
	}
}

func TestLoadSourcesFromJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sources.json")
	content := `[{"name":"Feed A","url":"https://a.example/rss"},{"name":"Feed B","url":"https://b.example/rss"}]`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	srcs, err := LoadSources(path)
	if err != nil {
		t.Fatalf("LoadSources(json): %v", err)
	}
	if len(srcs) != 2 || srcs[0].Name() != "Feed A" || srcs[1].Name() != "Feed B" {
		t.Fatalf("srcs: %d %v", len(srcs), srcs)
	}
}

func TestLoadSourcesFromExtendedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sources.json")
	content := `[
		{"name":"Feed A","url":"https://a.example/rss","adapter":"rss","source_kind":"rss","source_role":"official","max_items_per_run":2},
		{"name":"Anthropic News","url":"https://www.anthropic.com/news","adapter":"anthropic_news_html","source_kind":"html","source_role":"official"},
		{"name":"AIHOT","url":"https://aihot.news/api/v1/items","adapter":"aihot_v1_items","source_kind":"aihot","source_role":"discovery","max_secondary_per_run":10},
		{"name":"Disabled","url":"https://disabled.example/rss","enabled":false}
	]`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	srcs, err := LoadSources(path)
	if err != nil {
		t.Fatalf("LoadSources(json): %v", err)
	}
	if len(srcs) != 3 {
		t.Fatalf("want 3 enabled sources, got %d", len(srcs))
	}
	names := []string{srcs[0].Name(), srcs[1].Name(), srcs[2].Name()}
	want := []string{"Feed A", "Anthropic News", "AIHOT"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names: got %v want %v", names, want)
		}
	}
}

func TestNormalizeSourceConfigCompatibilityDefaults(t *testing.T) {
	n, err := normalizeSourceConfig(SourceConfig{Name: "Old", URL: "https://old.example/rss"}, 0)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if n.adapter != adapterRSS || n.sourceKind != "rss" || n.sourceRole != "discovery" || !n.enabled {
		t.Fatalf("old config defaults: %+v", n)
	}
}

func TestNormalizeSourceConfigRateLimit(t *testing.T) {
	n, err := normalizeSourceConfig(SourceConfig{Name: "Limited", URL: "https://limited.example/rss", RateLimit: "10/m"}, 0)
	if err != nil {
		t.Fatalf("normalize count/window rate limit: %v", err)
	}
	if n.rateLimit != 6*time.Second {
		t.Fatalf("count/window rate limit: got %s want 6s", n.rateLimit)
	}

	n, err = normalizeSourceConfig(SourceConfig{Name: "Limited", URL: "https://limited.example/rss", RateLimit: "250ms"}, 0)
	if err != nil {
		t.Fatalf("normalize duration rate limit: %v", err)
	}
	if n.rateLimit != 250*time.Millisecond {
		t.Fatalf("duration rate limit: got %s want 250ms", n.rateLimit)
	}

	if _, err := normalizeSourceConfig(SourceConfig{Name: "Bad", URL: "https://bad.example/rss", RateLimit: "soon"}, 0); err == nil {
		t.Fatal("invalid rate_limit should error")
	}
}

func TestBuildSourceWrapsRateLimitedSource(t *testing.T) {
	src, enabled, err := buildSource(SourceConfig{Name: "Limited", URL: "https://limited.example/rss", RateLimit: "20ms"}, 0)
	if err != nil {
		t.Fatalf("buildSource: %v", err)
	}
	if !enabled {
		t.Fatal("source should be enabled")
	}
	limited, ok := src.(*ingest.RateLimitedSource)
	if !ok {
		t.Fatalf("source should be rate limited, got %T", src)
	}
	if limited.Delay != 20*time.Millisecond {
		t.Fatalf("delay: got %s want 20ms", limited.Delay)
	}
	if limited.Name() != "Limited" {
		t.Fatalf("name should delegate to wrapped source, got %q", limited.Name())
	}
}

func TestLoadSourcesAppendsMPCorpus(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIHOT_MP_CORPUS_DIR", dir)
	srcs, err := LoadSources("")
	if err != nil {
		t.Fatalf("LoadSources: %v", err)
	}
	found := false
	for _, s := range srcs {
		if s.Name() == "WeChat MP" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a WeChat MP source when AIHOT_MP_CORPUS_DIR is set")
	}
}

func TestLoadSourcesNoMPWhenUnset(t *testing.T) {
	t.Setenv("AIHOT_MP_CORPUS_DIR", "") // explicitly empty
	srcs, err := LoadSources("")
	if err != nil {
		t.Fatalf("LoadSources: %v", err)
	}
	for _, s := range srcs {
		if s.Name() == "WeChat MP" {
			t.Fatal("no MP source expected when env unset")
		}
	}
}

func TestLoadSourcesRejectsBadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	_ = os.WriteFile(path, []byte(`{not json`), 0o644)
	if _, err := LoadSources(path); err == nil {
		t.Fatal("bad json should error")
	}
	if _, err := LoadSources(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing file should error")
	}
	// Entries missing name/url are rejected.
	path2 := filepath.Join(t.TempDir(), "empty.json")
	_ = os.WriteFile(path2, []byte(`[{"name":"","url":""}]`), 0o644)
	if _, err := LoadSources(path2); err == nil {
		t.Fatal("empty name/url should error")
	}
	if _, err := normalizeSourceConfig(SourceConfig{Name: "Bad", URL: "https://bad.example/rss", Adapter: "unknown"}, 0); err == nil {
		t.Fatal("unknown adapter should error")
	}
	if _, err := normalizeSourceConfig(SourceConfig{Name: "Bad", URL: "https://bad.example/rss", SourceRole: "vendor"}, 0); err == nil {
		t.Fatal("unknown source role should error")
	}
	if _, err := normalizeSourceConfig(SourceConfig{Name: "Bad", URL: "https://bad.example/rss", Adapter: adapterRSS, SourceKind: "html"}, 0); err == nil {
		t.Fatal("adapter/kind mismatch should error")
	}
}
