# 公众号 (WeChat MP) Content Source Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ingest AI-relevant 微信公众号 articles from the synced `wechat-corpus` into the aihot feed via a new `MPCorpusSource` that plugs into the existing ingest runner.

**Architecture:** A Mac cron rsyncs the corpus `.md` files to Melos. On Melos, `MPCorpusSource` (an `ingest.Source`) walks the synced dir, parses each file's frontmatter, keeps AI-relevant articles via a keyword pre-filter, and yields `RawItem`s that flow through the unchanged enrich → score → 精选/全部 pipeline. A one-off `cmd/importmp` backfills the ~3-month history (bypassing the runner's 14-day age gate).

**Tech Stack:** Go (stdlib only — no new deps), pgx/pgxpool, the existing `ingest.Source` interface and `raw_items` pipeline.

**Build/test invocation (toolchain footgun):** always `GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org` for `go build`/`go test`. Cross-compile for Melos adds `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`.

**Reference — existing `RawItem` (internal/ingest/rawitem.go):**
```go
type RawItem struct {
	ID          string     // = RawID(URL)
	Source      string     // human source name
	SourceKind  string     // "rss" | "mp" | ...
	URL         string
	Title       string
	PublishedAt *time.Time
	RawContent  *string
	ImageURL    *string
	VideoURL    *string
}
func RawID(url string) string // sha256 hex of the URL
```
**Note (image):** WeChat article images are hotlink-protected and won't render from our origin, so MP items are intentionally thumbnail-less (`ImageURL` left nil). The feed already hides broken images. Not worth extracting.

---

### Task 1: Frontmatter parser

**Files:**
- Create: `server/internal/ingest/mpsource.go`
- Test: `server/internal/ingest/mpsource_test.go`

- [ ] **Step 1: Write the failing test**

Create `server/internal/ingest/mpsource_test.go`:
```go
package ingest

import "testing"

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
	// No frontmatter block.
	if _, ok := parseMPFrontmatter([]byte("# just a body")); ok {
		t.Fatal("no frontmatter should be rejected")
	}
	// Missing url.
	noURL := "---\ntitle: X\nmp_name: Y\n---\nbody"
	if _, ok := parseMPFrontmatter([]byte(noURL)); ok {
		t.Fatal("missing url should be rejected")
	}
	// Missing title.
	noTitle := "---\nurl: https://a/b\n---\nbody"
	if _, ok := parseMPFrontmatter([]byte(noTitle)); ok {
		t.Fatal("missing title should be rejected")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/ingest/ -run TestParseMPFrontmatter`
Expected: FAIL — `undefined: parseMPFrontmatter`.

- [ ] **Step 3: Write minimal implementation**

Create `server/internal/ingest/mpsource.go`:
```go
package ingest

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// mpArticle is the subset of a corpus .md's frontmatter we need, plus its body.
type mpArticle struct {
	URL         string
	MPName      string
	Title       string
	PublishTime string
	Body        string
}

// parseMPFrontmatter extracts the scalar frontmatter fields we need (url,
// mp_name, title, publish_time) and the markdown body from a corpus .md file.
// It reads only top-level "key: value" lines in the leading --- block, ignoring
// YAML folded continuations (e.g. the multi-line description, which we don't
// need — the full body is RawContent). ok is false when the file has no
// frontmatter block or is missing url/title.
func parseMPFrontmatter(data []byte) (mpArticle, bool) {
	s := string(data)
	if !strings.HasPrefix(s, "---") {
		return mpArticle{}, false
	}
	rest := strings.TrimPrefix(s[len("---"):], "\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return mpArticle{}, false
	}
	front := rest[:end]
	body := strings.TrimPrefix(rest[end+len("\n---"):], "\n")

	var a mpArticle
	a.Body = strings.TrimSpace(body)
	for _, line := range strings.Split(front, "\n") {
		// Only top-level scalars: key must start at column 0 (skip folded
		// continuation lines, which are indented).
		if line == "" || line[0] == ' ' || line[0] == '\t' {
			continue
		}
		colon := strings.Index(line, ": ")
		if colon < 0 {
			continue
		}
		key := line[:colon]
		val := strings.Trim(strings.TrimSpace(line[colon+2:]), `'"`)
		switch key {
		case "url":
			a.URL = val
		case "mp_name":
			a.MPName = val
		case "title":
			a.Title = val
		case "publish_time":
			a.PublishTime = val
		}
	}
	if a.URL == "" || a.Title == "" {
		return mpArticle{}, false
	}
	return a, true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/ingest/ -run TestParseMPFrontmatter`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/ingest/mpsource.go server/internal/ingest/mpsource_test.go
git commit -m "feat: parse WeChat 公众号 corpus frontmatter"
```

---

### Task 2: AI keyword pre-filter

**Files:**
- Modify: `server/internal/ingest/mpsource.go`
- Test: `server/internal/ingest/mpsource_test.go`

- [ ] **Step 1: Write the failing test**

Append to `server/internal/ingest/mpsource_test.go`:
```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/ingest/ -run TestIsAIRelevant`
Expected: FAIL — `undefined: isAIRelevant`.

- [ ] **Step 3: Write minimal implementation**

Append to `server/internal/ingest/mpsource.go`:
```go
// aiKeywords is the coarse pre-filter vocabulary (all lowercase). Curated to be
// AI-specific — bare "ai" is excluded because it matches "detail"/"again"/etc.
// The LLM enrichment is the second, authoritative gate for 精选.
var aiKeywords = []string{
	"ai coding", "ai 编程", "ai编程", "vibe coding", "coding agent", "cursor",
	"harness", "spec-driven", "sdd", "agent", "智能体", "多智能体",
	"大模型", "大语言模型", "llm", "rag", "mcp", "claude", "gpt", "copilot",
	"通义", "prompt", "提示词", "多模态", "微调", "推理模型", "生成式", "aigc",
	"机器学习", "深度学习",
}

// aiLedeRunes bounds how much of the body we scan (its lede ≈ the description);
// scanning the full body would match a keyword in almost any tech article.
const aiLedeRunes = 300

// isAIRelevant reports whether the title or the article's lede contains an AI
// keyword.
func isAIRelevant(title, body string) bool {
	hay := strings.ToLower(title + "\n" + firstRunes(body, aiLedeRunes))
	for _, kw := range aiKeywords {
		if strings.Contains(hay, kw) {
			return true
		}
	}
	return false
}

func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/ingest/ -run TestIsAIRelevant`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/ingest/mpsource.go server/internal/ingest/mpsource_test.go
git commit -m "feat: AI keyword pre-filter for 公众号 articles"
```

---

### Task 3: MPCorpusSource (ingest.Source implementation)

**Files:**
- Modify: `server/internal/ingest/mpsource.go`
- Test: `server/internal/ingest/mpsource_test.go`

- [ ] **Step 1: Write the failing test**

Append to `server/internal/ingest/mpsource_test.go`:
```go
import (
	"context"
	"os"
	"path/filepath"
	"testing"
)
// (merge these imports into the file's existing import block)

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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/ingest/ -run TestMPCorpusSource`
Expected: FAIL — `undefined: NewMPCorpusSource`.

- [ ] **Step 3: Write minimal implementation**

Append to `server/internal/ingest/mpsource.go`:
```go
// MPCorpusSource ingests AI-relevant WeChat 公众号 articles from a synced corpus
// of .md files (one directory per 公众号). It is strictly read-only over the
// corpus. Per-file errors are skipped so one bad file never aborts the walk.
type MPCorpusSource struct {
	root  string
	label string
}

func NewMPCorpusSource(root, label string) *MPCorpusSource {
	return &MPCorpusSource{root: root, label: label}
}

func (s *MPCorpusSource) Name() string { return s.label }

func (s *MPCorpusSource) Fetch(ctx context.Context) ([]RawItem, error) {
	var out []RawItem
	err := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries, keep walking
		}
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		a, ok := parseMPFrontmatter(data)
		if !ok || !isAIRelevant(a.Title, a.Body) {
			return nil
		}
		out = append(out, s.toRaw(a))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *MPCorpusSource) toRaw(a mpArticle) RawItem {
	r := RawItem{
		ID:         RawID(a.URL),
		Source:     a.MPName,
		SourceKind: "mp",
		URL:        a.URL,
		Title:      a.Title,
	}
	if r.Source == "" {
		r.Source = s.label
	}
	if t, err := time.Parse(time.RFC3339, a.PublishTime); err == nil {
		r.PublishedAt = &t
	}
	if a.Body != "" {
		body := a.Body
		r.RawContent = &body
	}
	return r
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/ingest/`
Expected: PASS (all ingest tests, including prior tasks).

- [ ] **Step 5: Commit**

```bash
git add server/internal/ingest/mpsource.go server/internal/ingest/mpsource_test.go
git commit -m "feat: MPCorpusSource reads 公众号 corpus as an ingest source"
```

---

### Task 4: Wire MPCorpusSource into LoadSources

**Files:**
- Modify: `server/internal/pulse/sources.go`
- Test: `server/internal/pulse/sources_test.go`

- [ ] **Step 1: Write the failing test**

Append to `server/internal/pulse/sources_test.go`:
```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/pulse/ -run TestLoadSourcesAppendsMPCorpus`
Expected: FAIL — no MP source appended.

- [ ] **Step 3: Write minimal implementation**

In `server/internal/pulse/sources.go`, add `"os"` to imports (already imported), and change the tail of `LoadSources` from:
```go
	var out []ingest.Source
	for i, c := range configs {
		if c.Name == "" || c.URL == "" {
			return nil, fmt.Errorf("source %d: name and url are required", i)
		}
		out = append(out, ingest.NewRSSSource(c.Name, c.URL))
	}
	return out, nil
}
```
to:
```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go test ./internal/pulse/`
Expected: PASS (new tests + existing LoadSources tests unaffected — they don't set the env).

- [ ] **Step 5: Commit**

```bash
git add server/internal/pulse/sources.go server/internal/pulse/sources_test.go
git commit -m "feat: wire 公众号 corpus source into LoadSources via AIHOT_MP_CORPUS_DIR"
```

---

### Task 5: One-off historical backfill command

**Files:**
- Create: `server/cmd/importmp/main.go`
- Modify: `server/.gitignore` (if a `/importmp` binary-ignore pattern is needed — verify the existing ignore list already covers stray cmd binaries)

- [ ] **Step 1: Write the command**

Create `server/cmd/importmp/main.go`:
```go
// importmp is a one-off: it reads the whole synced WeChat 公众号 corpus, keeps
// the AI-relevant articles, and InsertRaws them into raw_items with NO age gate,
// so the pulse enrich pipeline picks up the ~3-month backlog (steady-state
// ingest only takes articles <14d old). Idempotent — safe to re-run.
//
//	AIHOT_DATABASE_URL=postgres://... ./importmp -corpus /root/aihot/corpus/wechat
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"aihot-server/internal/db"
	"aihot-server/internal/ingest"
)

func main() {
	corpus := flag.String("corpus", "/root/aihot/corpus/wechat", "corpus root dir")
	flag.Parse()

	dsn := os.Getenv("AIHOT_DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "AIHOT_DATABASE_URL not set")
		os.Exit(1)
	}
	ctx := context.Background()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "db:", err)
		os.Exit(1)
	}
	defer pool.Close()

	raw := ingest.NewRawStore(pool)
	if err := raw.EnsureSchema(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "schema:", err)
		os.Exit(1)
	}

	src := ingest.NewMPCorpusSource(*corpus, "WeChat MP")
	items, err := src.Fetch(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fetch:", err)
		os.Exit(1)
	}
	inserted := 0
	for _, it := range items {
		ok, err := raw.InsertRaw(ctx, it)
		if err != nil {
			fmt.Fprintf(os.Stderr, "insert %s: %v\n", it.ID, err)
			continue
		}
		if ok {
			inserted++
		}
	}
	fmt.Printf("importmp: %d AI-relevant articles, %d newly inserted\n", len(items), inserted)
}
```

- [ ] **Step 2: Verify it builds**

Run: `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go build ./cmd/importmp`
Expected: builds, no stray binary committed (build from repo root writes `importmp` into CWD — delete it; `.gitignore` should already ignore `/importmp`-style cmd binaries. If `git status` shows an untracked `server/importmp`, add `/importmp` to `server/.gitignore`).

- [ ] **Step 3: Commit**

```bash
git add server/cmd/importmp/main.go server/.gitignore
git commit -m "feat: importmp one-off backfill for 公众号 history"
```

---

### Task 6: Mac → Melos sync script

**Files:**
- Create: `server/scripts/sync-wechat-corpus.sh`

- [ ] **Step 1: Write the script**

Create `server/scripts/sync-wechat-corpus.sh`:
```bash
#!/usr/bin/env bash
# Mirror the Mac-side 公众号 corpus to Melos so the aihot MPCorpusSource can read
# it. Read-only w.r.t. the corpus (never writes back). Run from the Mac (cron).
#
#   ./sync-wechat-corpus.sh [SRC] [DST]
# defaults: /Users/didi/wechat-corpus/  ->  melos:/root/aihot/corpus/wechat/
set -euo pipefail
SRC="${1:-/Users/didi/wechat-corpus/}"
DST="${2:-melos:/root/aihot/corpus/wechat/}"
ssh -o ClearAllForwardings=yes melos "mkdir -p /root/aihot/corpus/wechat"
rsync -az --delete -e "ssh -o ClearAllForwardings=yes" \
  --include='*/' --include='*.md' --exclude='*' \
  "$SRC" "$DST"
echo "synced $SRC -> $DST"
```

- [ ] **Step 2: Make executable + verify**

Run: `chmod +x server/scripts/sync-wechat-corpus.sh && bash -n server/scripts/sync-wechat-corpus.sh`
Expected: no syntax errors.

- [ ] **Step 3: Commit**

```bash
git add server/scripts/sync-wechat-corpus.sh
git commit -m "chore: Mac->Melos rsync script for 公众号 corpus"
```

---

### Task 7: Full build + test gate

- [ ] **Step 1: Build everything**

Run: `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go build ./...`
Expected: no output (success).

- [ ] **Step 2: Full test suite**

Run: `cd server && GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org make test`
Expected: all packages `ok`.

---

## Deployment (post-implementation, run manually)

1. Cross-compile the server binary and `importmp`:
   `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOTOOLCHAIN=go1.25.5 GOSUMDB=sum.golang.org go build -o <scratch>/server . && ... -o <scratch>/importmp ./cmd/importmp`
2. Create the corpus dir on Melos and run the sync from the Mac:
   `bash server/scripts/sync-wechat-corpus.sh`
3. Set `AIHOT_MP_CORPUS_DIR=/root/aihot/corpus/wechat` in the supervisor env (and the pulse cron env) on Melos, so steady-state pulses pick up new MP articles.
4. Ship binaries: `supervisorctl stop aihot-server` → scp server → start; scp `importmp` to `/root/aihot/bin/`.
5. Backfill history: `AIHOT_DATABASE_URL=postgres://aihot:aihot@localhost:5432/aihot /root/aihot/bin/importmp -corpus /root/aihot/corpus/wechat`.
6. Trigger a pulse (or wait for cron) to enrich the new `raw_items`; verify MP items appear via the API (`source` = a 公众号 name) and in the feed.

---

## Self-Review

**Spec coverage:** Sync (Task 6), MPCorpusSource incl. frontmatter parse + mapping (Tasks 1,3), pre-filter (Task 2), pulse wiring (Task 4), historical backfill (Task 5), enrich unchanged (no task needed), testing (Tasks 1–4). Media deliberately dropped (documented: weixin hotlink block). ✎ All spec sections covered.

**Placeholder scan:** No TBD/TODO/"handle errors" — every code step is complete.

**Type consistency:** `parseMPFrontmatter([]byte)(mpArticle,bool)`, `isAIRelevant(title,body string)bool`, `firstRunes(string,int)string`, `NewMPCorpusSource(root,label string)*MPCorpusSource`, `(*MPCorpusSource).Fetch/Name/toRaw` — names consistent across tasks. `SourceKind="mp"` and label `"WeChat MP"` consistent between source, LoadSources, importmp. Env var `AIHOT_MP_CORPUS_DIR` consistent (Task 4) with the corpus path `/root/aihot/corpus/wechat` (Tasks 5,6, deployment).
