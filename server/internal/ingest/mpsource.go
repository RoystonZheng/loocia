package ingest

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// mpArticle is the subset of a corpus .md's frontmatter we need, plus its body.
type mpArticle struct {
	URL         string
	MPName      string
	Title       string
	PublishTime string
	ImageURL    string
	Body        string
}

var markdownImageRe = regexp.MustCompile(`!\[[^\]]*\]\(\s*<?(https?://[^)\s>]+)>?`)

func firstMarkdownImage(body string) string {
	m := markdownImageRe.FindStringSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	return m[1]
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
		case "image_url", "pic_url":
			a.ImageURL = val
		}
	}
	// The domestic corpus already contains the WeChat cover as its first
	// Markdown image. Prefer that local evidence over fetching mp.weixin.qq.com
	// from the overseas serving host, which receives a verification page with no
	// og:image. Explicit frontmatter remains the stronger source when present.
	if a.ImageURL == "" {
		a.ImageURL = firstMarkdownImage(a.Body)
	}
	if a.URL == "" || a.Title == "" {
		return mpArticle{}, false
	}
	return a, true
}

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
	if strings.HasPrefix(a.ImageURL, "http://") || strings.HasPrefix(a.ImageURL, "https://") {
		imageURL := a.ImageURL
		r.ImageURL = &imageURL
	}
	return r
}
