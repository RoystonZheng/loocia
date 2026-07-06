package detailpage

import (
	"html/template"
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
	return vm
}

// beijing is UTC+8 fixed (no DST) — matches the front-end formatBeijingTime.
var beijing = time.FixedZone("CST", 8*3600)

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
// dark-mode aware, green accent matching the SPA.
var pageTemplate = template.Must(template.New("item").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>{{.Title}} · AI HOT（内网）</title>
<style>
:root { color-scheme: light dark; }
body { max-width: 720px; margin: 0 auto; padding: 32px 18px; font: 16px/1.6 system-ui, sans-serif; color: #1a1a1a; background: #fff; }
a { color: #1a7f5a; }
.back { display: inline-block; margin-bottom: 20px; text-decoration: none; font-size: .9rem; }
h1 { font-size: 1.6rem; line-height: 1.3; margin: 0 0 6px; }
.title-en { color: #888; font-size: 1rem; font-weight: 400; margin: 0 0 14px; }
.meta { display: flex; flex-wrap: wrap; gap: 10px; align-items: center; font-size: .85rem; color: #777; margin-bottom: 20px; }
.cat { background: #eef6f2; color: #1a7f5a; padding: 1px 8px; border-radius: 999px; }
.score { color: #1a7f5a; font-weight: 600; }
.summary { font-size: 1.05rem; margin: 0 0 28px; }
.readmore { display: inline-block; padding: 10px 18px; background: #1a7f5a; color: #fff; border-radius: 8px; text-decoration: none; }
.note { margin-top: 24px; font-size: .8rem; color: #aaa; }
@media (prefers-color-scheme: dark) {
  body { background: #16171d; color: #e6e6ea; }
  .title-en, .meta, .note { color: #9aa; }
  .cat { background: #14342a; }
}
</style>
</head>
<body>
<a class="back" href="/">← 返回 AI HOT</a>
<h1>{{.Title}}</h1>
{{if .TitleEN}}<p class="title-en">{{.TitleEN}}</p>{{end}}
<div class="meta">
  <span>{{.Source}}</span>
  {{if .Category}}<span class="cat">{{.Category}}</span>{{end}}
  {{if .PublishedAt}}<span>{{.PublishedAt}}</span>{{end}}
  {{if .HasScore}}<span class="score">{{.Score}}</span>{{end}}
</div>
{{if .Summary}}<p class="summary">{{.Summary}}</p>{{end}}
<a class="readmore" href="{{.URL}}" target="_blank" rel="noopener nofollow">阅读原文 →</a>
<p class="note">本页为站内中文精选呈现；正文版权归原信源所有，点击「阅读原文」查看完整内容。</p>
</body>
</html>`))

var notFoundTemplate = template.Must(template.New("404").Parse(`<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><meta name="robots" content="noindex"><title>未找到 · AI HOT（内网）</title>
<style>body{max-width:600px;margin:0 auto;padding:48px 18px;font:16px/1.6 system-ui,sans-serif;text-align:center;color-scheme:light dark;}a{color:#1a7f5a;}</style>
</head>
<body>
<h1>未找到该资讯</h1>
<p>这条内容可能已下线或不存在。</p>
<a href="/">← 返回 AI HOT</a>
</body>
</html>`))
