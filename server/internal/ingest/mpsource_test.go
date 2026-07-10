package ingest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

const sampleMP = `---
id: 3264589119-2247696119_1
mp_id: MP_WXS_3264589119
mp_name: 腾讯云开发者
title: Loop Engineering又是啥？一文讲清企业Agent落地
url: https://mp.weixin.qq.com/s/3Zbx4RHB4fOdomI5aA_wIQ
publish_time: '2026-07-02T08:45:00+08:00'
description: 关注腾讯云开发者，一手技术干货提前解锁
  这是折行的第二行，应被忽略
crawled_at: '2026-07-02T11:44:53+08:00'
source: we-mp-rss
---

# Loop Engineering又是啥？

正文第一段。`

func TestParseMPFrontmatter(t *testing.T) {
	a, ok := parseMPFrontmatter([]byte(sampleMP))
	if !ok {
		t.Fatal("expected ok")
	}
	if a.URL != "https://mp.weixin.qq.com/s/3Zbx4RHB4fOdomI5aA_wIQ" {
		t.Fatalf("url: %q", a.URL)
	}
	if a.MPName != "腾讯云开发者" {
		t.Fatalf("mp_name: %q", a.MPName)
	}
	if a.Title != "Loop Engineering又是啥？一文讲清企业Agent落地" {
		t.Fatalf("title: %q", a.Title)
	}
	if a.PublishTime != "2026-07-02T08:45:00+08:00" { // quotes stripped
		t.Fatalf("publish_time: %q", a.PublishTime)
	}
	if a.Body == "" || a.Body[0] != '#' {
		t.Fatalf("body should start at markdown heading: %q", a.Body)
	}
}

func TestParseMPFrontmatterRejectsMissing(t *testing.T) {
	if _, ok := parseMPFrontmatter([]byte("# just a body")); ok {
		t.Fatal("no frontmatter should be rejected")
	}
	noURL := "---\ntitle: X\nmp_name: Y\n---\nbody"
	if _, ok := parseMPFrontmatter([]byte(noURL)); ok {
		t.Fatal("missing url should be rejected")
	}
	noTitle := "---\nurl: https://a/b\n---\nbody"
	if _, ok := parseMPFrontmatter([]byte(noTitle)); ok {
		t.Fatal("missing title should be rejected")
	}
}

func TestIsAIRelevant(t *testing.T) {
	cases := []struct {
		title, body string
		want        bool
	}{
		{"用 Claude 做 Vibe Coding 实战", "", true},
		{"Harness Engineering 工程化落地", "", true},
		{"大模型推理优化", "", true},
		{"普通标题", "文章开头就聊到 AI Coding 的实践", true}, // keyword in lede
		{"QQ音乐服务端容量治理", "纯后端容量规划，与智能无关的运维话题细节展开。", false},
		{"前端性能优化实践", "首屏加载与打包体积压缩。", false},
	}
	for _, c := range cases {
		if got := isAIRelevant(c.title, c.body); got != c.want {
			t.Errorf("isAIRelevant(%q,…)=%v want %v", c.title, got, c.want)
		}
	}
}

func writeMP(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMPCorpusSourceFetch(t *testing.T) {
	root := t.TempDir()
	acct := filepath.Join(root, "腾讯云开发者")
	writeMP(t, acct, "a.md", sampleMP) // AI-relevant (title has Agent)
	writeMP(t, acct, "b.md", `---
title: QQ音乐容量治理
url: https://mp.weixin.qq.com/s/irrelevant
mp_name: 腾讯云开发者
publish_time: '2026-07-01T00:00:00+08:00'
---

纯运维话题。`) // not AI-relevant
	writeMP(t, acct, "notes.txt", "ignored, not .md")

	src := NewMPCorpusSource(root, "WeChat MP")
	if src.Name() != "WeChat MP" {
		t.Fatalf("name: %q", src.Name())
	}
	items, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 AI-relevant item, got %d", len(items))
	}
	it := items[0]
	if it.SourceKind != "mp" {
		t.Fatalf("source_kind: %q", it.SourceKind)
	}
	if it.Source != "腾讯云开发者" {
		t.Fatalf("source: %q", it.Source)
	}
	if it.ID != RawID(it.URL) {
		t.Fatalf("id must be RawID(url)")
	}
	if it.PublishedAt == nil {
		t.Fatal("publish time should parse")
	}
	if it.RawContent == nil || *it.RawContent == "" {
		t.Fatal("body should be RawContent")
	}
}

func TestMPCorpusSourceEmptyDir(t *testing.T) {
	src := NewMPCorpusSource(t.TempDir(), "WeChat MP")
	items, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("want 0, got %d", len(items))
	}
}
