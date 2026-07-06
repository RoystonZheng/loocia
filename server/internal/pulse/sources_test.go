package pulse

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSourcesDefaults(t *testing.T) {
	srcs, err := LoadSources("")
	if err != nil {
		t.Fatalf("LoadSources(\"\"): %v", err)
	}
	if len(srcs) < 2 {
		t.Fatalf("want >=2 default sources, got %d", len(srcs))
	}
	names := map[string]bool{}
	for _, s := range srcs {
		names[s.Name()] = true
	}
	if !names["OpenAI Blog"] || !names["Google AI Blog"] {
		t.Fatalf("default names: %v", names)
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
}
