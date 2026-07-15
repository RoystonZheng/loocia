package detailpage

import (
	"html/template"
	"net/url"
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
	Title          string
	TitleEN        string        // "" when absent
	Summary        string        // "" when absent
	Reason         string        // 精选理由, "" when absent
	Body           template.HTML // rendered 原文, "" when absent
	BodyOriginal   template.HTML // 英文原文, 仅 rss+已翻译时非空
	HasTranslation bool          // true → 展示中文 body + 可切回英文原文
	Source         string
	URL            string
	Domain         string // host of URL, for the 阅读原文·domain label
	ExportURL      string // permalink + ?format=md
	Selected       bool
	Category       string // localized label, "" when absent
	PublishedAt    string // "YYYY-MM-DD HH:MM" Beijing, "" when absent
	Score          string // "" when absent (tier letter S/A/B/C/D)
	Tier           string // same letter, for the data-tier styling hook
	HasScore       bool
	ImageURL       string // "" when absent
	VideoURL       string // "" when absent
	VideoEmbed     bool   // true → render as <iframe> (player/embed), else <video>
	ID             string // item id, for the retranslate JS call
	BodyCNModel    string // translation model name (HasTranslation only), "" → shown as "AI"
	CanRetranslate bool   // rss + has body + translation missing → show retry button
}

func toViewModel(it *items.Item) viewModel {
	vm := viewModel{
		ID:        it.ID,
		Title:     it.Title,
		Source:    it.Source,
		URL:       it.URL,
		Domain:    hostOf(it.URL),
		ExportURL: it.Permalink + "?format=md",
		Selected:  it.Selected,
	}
	if it.TitleEN != nil {
		vm.TitleEN = *it.TitleEN
	}
	if it.Summary != nil {
		vm.Summary = *it.Summary
	}
	if it.Reason != nil {
		vm.Reason = *it.Reason
	}
	if it.SourceKind == "rss" && it.BodyCN != nil && strings.TrimSpace(*it.BodyCN) != "" && it.Body != nil {
		// English source with a translation: show Chinese by default, keep the
		// English original one toggle away.
		vm.Body = renderBody("rss", *it.BodyCN)
		vm.BodyOriginal = renderBody("rss", *it.Body)
		vm.HasTranslation = true
		if it.BodyCNModel != nil && strings.TrimSpace(*it.BodyCNModel) != "" {
			vm.BodyCNModel = *it.BodyCNModel
		} else {
			vm.BodyCNModel = "AI"
		}
	} else if it.Body != nil {
		vm.Body = renderBody(it.SourceKind, *it.Body)
	}
	vm.CanRetranslate = it.SourceKind == "rss" && it.Body != nil &&
		(it.BodyCN == nil || strings.TrimSpace(*it.BodyCN) == "")
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
		vm.Score = scoreLetter(*it.Score)
		vm.Tier = vm.Score
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

// hostOf returns the bare host of a URL (no scheme/path), "" if unparseable.
func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
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

// scoreLetter maps a 1-5 tier to its display letter (5→S … 1→D), clamped.
func scoreLetter(n int) string {
	letters := []string{"D", "C", "B", "A", "S"}
	if n < 1 {
		n = 1
	}
	if n > 5 {
		n = 5
	}
	return letters[n-1]
}

// pageTemplate is self-contained (inline CSS+JS, no external assets), noindex,
// theme-aware (shares the SPA's aihot-theme localStorage key), cool-blue accent.
var pageTemplate = template.Must(template.New("item").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>{{.Title}} · AI Cool（内网）</title>
<script>
(function(){var K='aihot-theme';function sd(){return typeof matchMedia!=='undefined'&&matchMedia('(prefers-color-scheme: dark)').matches;}
function rd(){try{return localStorage.getItem(K)||'system';}catch(e){return 'system';}}
function ap(t){var e=t==='system'?(sd()?'dark':'light'):t;document.documentElement.dataset.theme=e;}
function mk(t){['light','dark','system'].forEach(function(k){var el=document.getElementById('th-'+k);if(el)el.setAttribute('aria-pressed',String(k===t));});}
window.__setTheme=function(t){try{localStorage.setItem(K,t);}catch(e){}ap(t);mk(t);};
window.__toggleOrig=function(){var cn=document.getElementById('orig-cn'),en=document.getElementById('orig-en'),b=document.getElementById('tr-toggle');if(!cn||!en||!b)return;var showEn=en.style.display==='none';en.style.display=showEn?'':'none';cn.style.display=showEn?'none':'';b.textContent=showEn?'看中文翻译':'看英文原文';};
window.__retranslate=function(id){var b=document.getElementById('tr-retry');if(b){b.disabled=true;b.textContent='翻译中…';}fetch('/items/'+id+'/retranslate',{method:'POST'}).then(function(r){return r.json();}).then(function(j){if(j&&j.ok){location.reload();}else{if(b){b.disabled=false;b.textContent='重试仍失败，可稍后再试';}}}).catch(function(){if(b){b.disabled=false;b.textContent='重试失败，可稍后再试';}});};
ap(rd());document.addEventListener('DOMContentLoaded',function(){mk(rd());});})();
</script>
<style>
:root{color-scheme:light dark;--bg:#f5f6f8;--card:#fff;--text:#1a1c20;--text-2:#5a6069;--muted:#8b929c;--border:#e6e8ec;--sidebar:#fff;--accent:#2f6bff;--accent-2:#1e5ae6;--accent-soft:#eaf0ff;--gold:#b8860b;--gold-soft:#fdf3d7;--chip:#eef0f4;}
:root[data-theme="dark"]{--bg:#0f1115;--card:#16181d;--text:#e6e8eb;--text-2:#b3b8bf;--muted:#7c828b;--border:#24272e;--sidebar:#121419;--accent:#5b8cff;--accent-2:#7aa0ff;--accent-soft:#17233f;--gold:#e6b84d;--gold-soft:#2a2412;--chip:#1c1f25;}
@media (prefers-color-scheme:dark){:root:not([data-theme]){--bg:#0f1115;--card:#16181d;--text:#e6e8eb;--text-2:#b3b8bf;--muted:#7c828b;--border:#24272e;--sidebar:#121419;--accent:#5b8cff;--accent-2:#7aa0ff;--accent-soft:#17233f;--gold:#e6b84d;--gold-soft:#2a2412;--chip:#1c1f25;}}
*{box-sizing:border-box;}
body{margin:0;font:16px/1.7 -apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif;color:var(--text);background:var(--bg);}
a{color:var(--accent);}
.shell{display:flex;min-height:100vh;}
.sidebar{width:220px;flex-shrink:0;background:var(--sidebar);border-right:1px solid var(--border);padding:22px 16px;position:sticky;top:0;height:100vh;display:flex;flex-direction:column;}
.logo{display:flex;align-items:center;gap:2px;font-weight:800;font-size:20px;letter-spacing:.5px;margin:4px 6px 26px;}
.logo .dot{color:var(--accent);}
.nav-label{font-size:12px;color:var(--muted);margin:0 8px 8px;}
.nav a{display:flex;align-items:center;gap:10px;padding:9px 10px;border-radius:9px;text-decoration:none;color:var(--text-2);font-size:14px;font-weight:600;}
.nav a:hover{background:var(--chip);color:var(--text);}
.theme{margin-top:auto;display:flex;gap:6px;padding:6px;}
.theme button{flex:1;padding:7px 0;border:1px solid var(--border);background:var(--card);border-radius:8px;color:var(--text-2);cursor:pointer;font-size:14px;}
.theme button[aria-pressed="true"]{border-color:var(--accent);color:var(--accent);}
.main{flex:1;min-width:0;display:flex;justify-content:center;padding:34px 28px 80px;}
.article{width:100%;max-width:720px;}
.topbar{display:flex;align-items:center;gap:10px;flex-wrap:wrap;margin-bottom:18px;}
.src{font-weight:700;font-size:14px;color:var(--text-2);}
.badge-sel{font-size:12px;font-weight:700;color:var(--gold);background:var(--gold-soft);border-radius:999px;padding:2px 10px;}
.badge-score{font-size:12px;font-weight:700;border-radius:999px;padding:2px 10px;letter-spacing:.3px;}
.badge-score[data-tier="S"]{color:#b7791f;background:#fff4d6;}
.badge-score[data-tier="A"]{color:var(--accent-2);background:var(--accent-soft);}
.badge-score[data-tier="B"]{color:#0f8a4f;background:#e6f7ee;}
.badge-score[data-tier="C"]{color:#6b7078;background:#eef0f2;}
.badge-score[data-tier="D"]{color:#8b9098;background:#f2f3f5;}
:root[data-theme="dark"] .badge-score[data-tier="S"]{color:#e5b95b;background:#2a2312;}
:root[data-theme="dark"] .badge-score[data-tier="B"]{color:#4fcf8e;background:#10261b;}
:root[data-theme="dark"] .badge-score[data-tier="C"]{color:#9096a0;background:#1c1f25;}
:root[data-theme="dark"] .badge-score[data-tier="D"]{color:#7c828b;background:#191c22;}
.export{margin-left:auto;font-size:13px;font-weight:600;color:var(--text-2);border:1px solid var(--border);background:var(--card);border-radius:8px;padding:6px 12px;text-decoration:none;}
.export:hover{border-color:var(--accent);color:var(--accent);}
h1{font-size:1.75rem;line-height:1.3;margin:0 0 8px;}
.title-en{color:var(--muted);font-size:1rem;font-weight:400;margin:0 0 12px;}
.meta{display:flex;flex-wrap:wrap;gap:12px;align-items:center;font-size:.85rem;color:var(--muted);margin-bottom:22px;}
.box{border:1px solid var(--border);border-radius:12px;padding:14px 18px;margin:0 0 16px;background:var(--card);}
.box .lbl{font-size:12px;font-weight:700;margin:0 0 6px;letter-spacing:.5px;}
.box .txt{font-size:.98rem;color:var(--text);margin:0;}
.box-reason{border-color:var(--accent-soft);background:var(--accent-soft);}
.box-reason .lbl{color:var(--accent-2);}
.media{margin:8px 0 24px;}
.media img,.media video{width:100%;max-height:440px;object-fit:cover;border-radius:12px;display:block;background:var(--chip);}
.media .embed{position:relative;width:100%;aspect-ratio:16/9;border-radius:12px;overflow:hidden;background:#000;}
.media .embed iframe{position:absolute;inset:0;width:100%;height:100%;border:0;}
.orig-h{font-size:13px;font-weight:700;color:var(--muted);letter-spacing:1px;margin:28px 0 12px;padding-bottom:8px;border-bottom:1px solid var(--border);}
.orig{font-size:1.02rem;line-height:1.85;color:var(--text);}
.tr-toggle{margin-left:8px;font-size:12px;padding:2px 8px;border:1px solid var(--border);border-radius:999px;background:var(--card);color:var(--accent-2);cursor:pointer;}
.back{font-size:13px;color:var(--text-2);text-decoration:none;font-weight:600;}
.back:hover{color:var(--text);}
.tr-note{font-size:12px;color:var(--muted);margin-top:8px;}
#tr-retry{margin-top:8px;font-size:13px;padding:5px 12px;border:1px solid var(--border);border-radius:8px;background:var(--card);color:var(--accent-2);cursor:pointer;}
#tr-retry:disabled{opacity:.6;cursor:default;}
.orig p{margin:0 0 1.1em;}
.orig h1,.orig h2,.orig h3{font-size:1.15rem;margin:1.4em 0 .6em;line-height:1.4;}
.orig img{max-width:100%;height:auto;border-radius:8px;margin:.6em 0;}
.orig a{word-break:break-all;}
.orig pre{overflow-x:auto;background:var(--chip);padding:12px;border-radius:8px;}
.orig code{background:var(--chip);padding:1px 5px;border-radius:4px;}
.orig blockquote{margin:0 0 1em;padding-left:14px;border-left:3px solid var(--border);color:var(--text-2);}
.tags{display:flex;flex-wrap:wrap;gap:8px;margin:24px 0;}
.tag{font-size:12px;color:var(--text-2);background:var(--chip);border-radius:6px;padding:4px 10px;}
.readmore{display:inline-block;padding:10px 18px;background:var(--accent);color:#fff;border-radius:8px;text-decoration:none;font-weight:600;}
.note{margin-top:26px;font-size:.8rem;color:var(--muted);}
@media (max-width:720px){
.shell{flex-direction:column;}
.sidebar{display:flex;width:auto;height:auto;position:sticky;top:0;z-index:20;flex-direction:row;align-items:center;gap:8px;border-right:none;border-bottom:1px solid var(--border);padding:8px 12px;overflow-x:auto;}
.logo{margin:0 8px 0 0;font-size:17px;}
.nav-label{display:none;}
.nav{display:flex;flex-direction:row;gap:2px;}
.nav a{padding:7px 10px;white-space:nowrap;}
.theme{margin:0 0 0 auto;padding:0;}
.main{padding:18px 14px 48px;}
.article{max-width:100%;}
}
</style>
</head>
<body>
<div class="shell">
  <aside class="sidebar">
    <div class="logo">AI<span class="dot">◉</span>Cool</div>
    <div class="nav-label">内容</div>
    <nav class="nav">
      <a href="/">✦ 精选</a>
      <a href="/#all">≣ 全部 AI 动态</a>
      <a href="/#daily">▤ AI 日报</a>
      <a href="/#graph">❖ 图谱</a>
    </nav>
    <div class="theme" role="group" aria-label="主题">
      <button id="th-dark" title="深色" onclick="__setTheme('dark')">☾</button>
      <button id="th-system" title="跟随系统" onclick="__setTheme('system')">▢</button>
      <button id="th-light" title="浅色" onclick="__setTheme('light')">☀</button>
    </div>
  </aside>
  <main class="main">
    <article class="article">
      <div class="topbar">
        <a class="back" href="/">← 返回</a>
        <span class="src">{{.Source}}</span>
        {{if .Selected}}<span class="badge-sel">✦ 精选</span>{{end}}
        {{if .HasScore}}<span class="badge-score" data-tier="{{.Tier}}">{{.Score}}</span>{{end}}
        <a class="export" href="{{.ExportURL}}">导出 Markdown ↓</a>
      </div>
      <h1>{{.Title}}</h1>
      {{if .TitleEN}}<p class="title-en">{{.TitleEN}}</p>{{end}}
      <div class="meta">
        {{if .PublishedAt}}<span>{{.PublishedAt}}</span>{{end}}
        <a href="{{.URL}}" target="_blank" rel="noopener nofollow">阅读原文{{if .Domain}} · {{.Domain}}{{end}}</a>
      </div>
      {{if .Reason}}<div class="box box-reason"><p class="lbl">精选理由</p><p class="txt">{{.Reason}}</p></div>{{end}}
      {{if .Summary}}<div class="box"><p class="lbl">AI 摘要</p><p class="txt">{{.Summary}}</p></div>{{end}}
      {{if .VideoURL}}
      <div class="media">
        {{if .VideoEmbed}}<div class="embed"><iframe src="{{.VideoURL}}" allowfullscreen loading="lazy"></iframe></div>
        {{else}}<video controls preload="metadata"{{if .ImageURL}} poster="{{.ImageURL}}"{{end}} src="{{.VideoURL}}"></video>{{end}}
      </div>
      {{else if .ImageURL}}
      <div class="media"><img src="{{.ImageURL}}" alt="" loading="lazy" referrerpolicy="no-referrer" onerror="this.parentNode.style.display='none'"></div>
      {{end}}
      {{if .Body}}
      <div class="orig-h">原文{{if .HasTranslation}} <button type="button" class="tr-toggle" id="tr-toggle" onclick="__toggleOrig()">看英文原文</button>{{end}}</div>
      <div class="orig" id="orig-cn">{{.Body}}</div>
      {{if .HasTranslation}}<div class="orig" id="orig-en" style="display:none">{{.BodyOriginal}}</div>
      <p class="tr-note">本文由 AI（{{.BodyCNModel}}）翻译</p>{{end}}
      {{if .CanRetranslate}}<button type="button" id="tr-retry" onclick="__retranslate('{{.ID}}')">翻译失败 · 点此重试翻译</button>{{end}}
      {{end}}
      <div class="tags">
        {{if .Category}}<span class="tag">#{{.Category}}</span>{{end}}
      </div>
      <a class="readmore" href="{{.URL}}" target="_blank" rel="noopener nofollow">阅读原文 →</a>
      <p class="note">本页为站内中文呈现（内网 noindex）；正文版权归原信源所有，请以「阅读原文」为准。</p>
    </article>
  </main>
</div>
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
