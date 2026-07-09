package detailpage

import (
	"html/template"
	"strings"
	"time"

	"aihot-server/internal/items"
)

var categoryLabels = map[string]string{
	items.CategoryAIModels:   "模型发布/更新",
	items.CategoryAIProducts: "产品发布/更新",
	items.CategoryIndustry:   "行业动态",
	items.CategoryPaper:      "论文研究",
	items.CategoryTip:        "技巧与观点",
}

// viewModel is the flattened, render-ready shape (no nil pointers reach the template).
type viewModel struct {
	Title       string
	TitleEN     string // "" when absent
	Summary     string // "" when absent
	Source      string
	URL         string
	Category    string // localized label, "" when absent
	PublishedAt string // "YYYY-MM-DD HH:MM" Beijing, "" when absent
	Score       string // "" when absent
	HasScore    bool
	ImageURL    string // "" when absent
	VideoURL    string // "" when absent
	VideoEmbed  bool   // true → render as <iframe> (player/embed), else <video>
}

func toViewModel(it *items.Item) viewModel {
	vm := viewModel{Title: it.Title, Source: it.Source, URL: it.URL}
	if it.TitleEN != nil {
		vm.TitleEN = *it.TitleEN
	}
	if it.Summary != nil {
		vm.Summary = *it.Summary
	}
	if it.Category != nil {
		if lbl, ok := categoryLabels[*it.Category]; ok {
			vm.Category = lbl
		} else {
			vm.Category = *it.Category
		}
	}
	if it.PublishedAt != nil {
		vm.PublishedAt = it.PublishedAt.In(beijing).Format("2006-01-02 15:04")
	}
	if it.Score != nil {
		vm.Score = itoa(*it.Score)
		vm.HasScore = true
	}
	if it.ImageURL != nil {
		vm.ImageURL = *it.ImageURL
	}
	if it.VideoURL != nil {
		vm.VideoURL = *it.VideoURL
		vm.VideoEmbed = !isVideoFile(vm.VideoURL)
	}
	return vm
}

// beijing is UTC+8 fixed (no DST) — matches the front-end formatBeijingTime.
var beijing = time.FixedZone("CST", 8*3600)

// isVideoFile reports whether the URL points at a directly-playable media file
// (renderable via <video>) rather than an embed/player page (needs <iframe>).
func isVideoFile(u string) bool {
	q := u
	if i := strings.IndexByte(q, '?'); i >= 0 {
		q = q[:i]
	}
	q = strings.ToLower(q)
	for _, ext := range []string{".mp4", ".webm", ".mov", ".m4v", ".ogv"} {
		if strings.HasSuffix(q, ext) {
			return true
		}
	}
	return false
}

func itoa(i int) string {
	// small helper to avoid importing strconv just for one call site
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}

// pageTemplate is self-contained (inline CSS, no external assets), noindex,
// dark-mode aware, cool-blue accent matching the SPA.
var pageTemplate = template.Must(template.New("item").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>{{.Title}} · AI Cool（内网）</title>
<style>
:root { color-scheme: light dark; }
body { max-width: 720px; margin: 0 auto; padding: 32px 18px; font: 16px/1.6 system-ui, sans-serif; color: #1a1a1a; background: #fff; }
a { color: #2f6bff; }
.back { display: inline-block; margin-bottom: 20px; text-decoration: none; font-size: .9rem; }
h1 { font-size: 1.6rem; line-height: 1.3; margin: 0 0 6px; }
.title-en { color: #888; font-size: 1rem; font-weight: 400; margin: 0 0 14px; }
.meta { display: flex; flex-wrap: wrap; gap: 10px; align-items: center; font-size: .85rem; color: #777; margin-bottom: 20px; }
.cat { background: #eaf0ff; color: #1e5ae6; padding: 1px 8px; border-radius: 999px; }
.score { color: #2f6bff; font-weight: 600; }
.media { margin: 0 0 24px; }
.media img, .media video { width: 100%; max-height: 420px; object-fit: cover; border-radius: 12px; display: block; background: #f0f2f6; }
.media .embed { position: relative; width: 100%; aspect-ratio: 16/9; border-radius: 12px; overflow: hidden; background: #000; }
.media .embed iframe { position: absolute; inset: 0; width: 100%; height: 100%; border: 0; }
.summary { font-size: 1.05rem; margin: 0 0 28px; }
.readmore { display: inline-block; padding: 10px 18px; background: #2f6bff; color: #fff; border-radius: 8px; text-decoration: none; }
.note { margin-top: 24px; font-size: .8rem; color: #aaa; }
@media (prefers-color-scheme: dark) {
  body { background: #16171d; color: #e6e6ea; }
  a, .score { color: #5b8cff; }
  .title-en, .meta, .note { color: #9aa; }
  .cat { background: #17233f; color: #7aa0ff; }
  .readmore { background: #2f6bff; }
  .media img, .media video { background: #23252e; }
}
</style>
</head>
<body>
<a class="back" href="/">← 返回 AI Cool</a>
<h1>{{.Title}}</h1>
{{if .TitleEN}}<p class="title-en">{{.TitleEN}}</p>{{end}}
<div class="meta">
  <span>{{.Source}}</span>
  {{if .Category}}<span class="cat">{{.Category}}</span>{{end}}
  {{if .PublishedAt}}<span>{{.PublishedAt}}</span>{{end}}
  {{if .HasScore}}<span class="score">{{.Score}}</span>{{end}}
</div>
{{if .VideoURL}}
<div class="media">
  {{if .VideoEmbed}}<div class="embed"><iframe src="{{.VideoURL}}" allowfullscreen loading="lazy"></iframe></div>
  {{else}}<video controls preload="metadata"{{if .ImageURL}} poster="{{.ImageURL}}"{{end}} src="{{.VideoURL}}"></video>{{end}}
</div>
{{else if .ImageURL}}
<div class="media"><img src="{{.ImageURL}}" alt="" loading="lazy"></div>
{{end}}
{{if .Summary}}<p class="summary">{{.Summary}}</p>{{end}}
<a class="readmore" href="{{.URL}}" target="_blank" rel="noopener nofollow">阅读原文 →</a>
<p class="note">本页为站内中文精选呈现；正文版权归原信源所有，点击「阅读原文」查看完整内容。</p>
</body>
</html>`))

var notFoundTemplate = template.Must(template.New("404").Parse(`<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><meta name="robots" content="noindex"><title>未找到 · AI Cool（内网）</title>
<style>body{max-width:600px;margin:0 auto;padding:48px 18px;font:16px/1.6 system-ui,sans-serif;text-align:center;color-scheme:light dark;}a{color:#2f6bff;}</style>
</head>
<body>
<h1>未找到该资讯</h1>
<p>这条内容可能已下线或不存在。</p>
<a href="/">← 返回 AI Cool</a>
</body>
</html>`))
