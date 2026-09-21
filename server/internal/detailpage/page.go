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
	// rss + 有原文 → 就能重译(不管当前有没有译文;已有译文也允许重译,修不完整的翻译)
	vm.CanRetranslate = it.SourceKind == "rss" && it.Body != nil
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
<title>{{.Title}} · AI Cool</title>
<script>
(function(){var K='aihot-theme';function sd(){return typeof matchMedia!=='undefined'&&matchMedia('(prefers-color-scheme: dark)').matches;}
function rd(){try{return localStorage.getItem(K)||'system';}catch(e){return 'system';}}
function ap(t){var e=t==='system'?(sd()?'dark':'light'):t;document.documentElement.dataset.theme=e;}
function mk(t){['light','dark','system'].forEach(function(k){var el=document.getElementById('th-'+k);if(el)el.setAttribute('aria-pressed',String(k===t));});}
window.__setTheme=function(t){try{localStorage.setItem(K,t);}catch(e){}ap(t);mk(t);};
window.__goBack=function(){if(document.referrer&&history.length>1){history.back();return false;}return true;};
window.__toggleOrig=function(){var cn=document.getElementById('orig-cn'),en=document.getElementById('orig-en'),b=document.getElementById('tr-toggle');if(!cn||!en||!b)return;var showEn=en.style.display==='none';en.style.display=showEn?'':'none';cn.style.display=showEn?'none':'';b.textContent=showEn?'看中文翻译':'看英文原文';};
window.__retranslate=function(id){var bs=document.querySelectorAll('.tr-retry-btn');bs.forEach(function(b){b.disabled=true;b.textContent='翻译中…';});fetch('/items/'+id+'/retranslate',{method:'POST'}).then(function(r){return r.json();}).then(function(j){if(j&&j.ok){location.reload();}else{bs.forEach(function(b){b.disabled=false;b.textContent='重试仍失败';});}}).catch(function(){bs.forEach(function(b){b.disabled=false;b.textContent='重试失败';});});};
function fillSidebarCount(key,path){fetch(path).then(function(r){return r.ok?r.json():null;}).then(function(body){var count=body&&body.errno===0&&body.data?body.data.count:null;var el=document.querySelector('[data-sidebar-count="'+key+'"]');if(el&&typeof count==='number'){el.textContent=String(count);el.hidden=false;}}).catch(function(){});}
ap(rd());document.addEventListener('DOMContentLoaded',function(){mk(rd());fillSidebarCount('discovered','/api/tools/items?status=discovered&take=1');fillSidebarCount('team','/api/tools/items?status=included&take=1');fillSidebarCount('configs','/api/tools/configs');});})();
</script>
<style>
@font-face{font-family:'Orbitron';font-weight:700;font-display:swap;src:url(data:font/woff2;base64,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) format('woff2');}
:root{color-scheme:light dark;--bg:#fff;--bg-soft:#f6f7f9;--sidebar:#f3f4f6;--text:#111827;--text-2:#4b5563;--muted:#8a94a3;--border:#dde1e6;--border-2:#cfd5de;--card:#fff;--accent:#285ee8;--accent-2:#1d4ed8;--accent-soft:#e8efff;--dot:#3b9dff;--chip:#f2f3f5;--gold:#f0a500;--gold-soft:#fff8e8;--sans:-apple-system,"PingFang SC","Microsoft YaHei",system-ui,"Segoe UI",Roboto,sans-serif;}
:root[data-theme="dark"]{--bg:#0f1115;--bg-soft:#14161b;--sidebar:#121419;--text:#e6e8eb;--text-2:#b3b8bf;--muted:#7c828b;--border:#24272e;--border-2:#2b2f37;--card:#16181d;--accent:#5b8cff;--accent-2:#7aa0ff;--accent-soft:#17233f;--dot:#4aa8ff;--chip:#1c1f25;--gold:#e0a83a;--gold-soft:#241f14;}
@media (prefers-color-scheme:dark){:root:not([data-theme]){--bg:#0f1115;--bg-soft:#14161b;--sidebar:#121419;--text:#e6e8eb;--text-2:#b3b8bf;--muted:#7c828b;--border:#24272e;--border-2:#2b2f37;--card:#16181d;--accent:#5b8cff;--accent-2:#7aa0ff;--accent-soft:#17233f;--dot:#4aa8ff;--chip:#1c1f25;--gold:#e0a83a;--gold-soft:#241f14;}}
*{box-sizing:border-box;}
body{margin:0;background:var(--bg);color:var(--text);font:15px/1.6 var(--sans);-webkit-font-smoothing:antialiased;text-rendering:optimizeLegibility;}
a{color:inherit;text-decoration:none;}
.shell{display:flex;min-height:100vh;}
.main{flex:1;min-width:0;max-width:1180px;padding:28px 40px 60px;background:var(--bg-soft);}
.sidebar{width:216px;flex-shrink:0;background:var(--sidebar);border-right:1px solid var(--border);padding:18px 14px;display:flex;flex-direction:column;gap:16px;position:sticky;top:0;height:100vh;}
.sb-logo{display:flex;align-items:center;gap:5px;font-family:'Orbitron',var(--sans);font-weight:700;font-size:22px;letter-spacing:.2px;padding:14px 14px;border:1px solid var(--border);border-radius:7px;background:var(--card);box-shadow:0 1px 0 rgba(17,24,39,.02);}
.sb-logo-hot{color:var(--accent);}
.sb-logo-mark{flex-shrink:0;display:block;}
.sb-logo small{margin-left:auto;color:var(--muted);font-family:var(--sans);font-size:10px;font-weight:700;letter-spacing:0;}
.sb-nav{display:flex;flex-direction:column;gap:14px;flex:1;}
.sb-nav-group{display:flex;flex-direction:column;gap:3px;}
.sb-group{font-size:12px;color:#6b7280;padding:0 10px 5px;letter-spacing:0;}
.sb-item{display:grid;grid-template-columns:18px minmax(0,1fr) auto;align-items:center;gap:10px;width:100%;padding:9px 10px;border:0;background:transparent;cursor:pointer;line-height:normal;border-radius:7px;color:var(--text-2);font-size:14px;text-align:left;text-decoration:none;}
.sb-item:hover{background:#eaedf2;color:var(--text);}
.sb-item.active{background:var(--accent-soft);color:var(--accent-2);font-weight:700;}
.sb-icon{width:18px;text-align:center;color:inherit;opacity:.75;}
.sb-badge{min-width:20px;height:20px;display:inline-flex;align-items:center;justify-content:center;border:1px solid var(--border);border-radius:999px;background:var(--card);color:#6b7280;font-size:11px;font-weight:700;padding:0 6px;}
.sb-foot{border-top:1px solid var(--border);padding:12px 10px 0;display:flex;flex-direction:column;gap:1px;font-size:12px;color:var(--muted);}
.sb-foot strong{color:var(--text-2);font-size:12px;}
.article{width:100%;max-width:820px;margin:0 auto;}
@media (max-width:900px){.main{padding:20px 16px 40px;}}
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
.tr-retry-btn:disabled{opacity:.6;cursor:default;}
.tr-relink{background:none;border:0;padding:0;font-size:inherit;color:var(--accent-2);text-decoration:underline;cursor:pointer;}
.orig p{margin:0 0 1.1em;}
.orig h1,.orig h2,.orig h3{font-size:1.15rem;margin:1.4em 0 .6em;line-height:1.4;}
.orig img{max-width:100%;height:auto;border-radius:8px;margin:.6em 0;}
.orig a{word-break:break-all;}
.orig pre{overflow-x:auto;background:var(--chip);padding:12px;border-radius:8px;}
.orig code{background:var(--chip);padding:1px 5px;border-radius:4px;}
.orig blockquote{margin:0 0 1em;padding-left:14px;border-left:3px solid var(--border);color:var(--text-2);}
.orig table{border-collapse:collapse;width:100%;margin:18px 0;font-size:.92rem;line-height:1.6;display:block;overflow-x:auto;}
.orig th,.orig td{border:1px solid var(--border);padding:8px 12px;text-align:left;vertical-align:top;}
.orig thead th{background:var(--chip);font-weight:700;}
.orig tbody tr:first-child td{background:var(--chip);font-weight:600;}
.orig tbody tr:nth-child(even):not(:first-child) td{background:color-mix(in srgb,var(--chip) 45%,transparent);}
.tags{display:flex;flex-wrap:wrap;gap:8px;margin:24px 0;}
.tag{font-size:12px;color:var(--text-2);background:var(--chip);border-radius:6px;padding:4px 10px;}
.readmore{display:inline-block;padding:10px 18px;background:var(--accent);color:#fff;border-radius:8px;text-decoration:none;font-weight:600;}
.note{margin-top:26px;font-size:.8rem;color:var(--muted);}
@media (max-width:600px){
.shell{flex-direction:column;}
.sidebar{width:auto;height:auto;position:sticky;top:0;z-index:20;flex-direction:row;align-items:center;gap:6px;border-right:none;border-bottom:1px solid var(--border);padding:8px 12px;overflow-x:auto;}
.sb-logo{margin:0 8px 0 0;padding:6px 8px;font-size:16px;border:0;}
.sb-logo small,.sb-foot{display:none;}
.sb-nav{flex-direction:row;flex:1;gap:2px;}
.sb-nav-group{flex-direction:row;gap:2px;}
.sb-group{display:none;}
.sb-item{display:inline-flex;padding:7px 10px;white-space:nowrap;}
.main{padding:16px 14px 40px;}
.article{max-width:100%;}
}
</style>
</head>
<body>
<div class="shell">
  <aside class="sidebar">
    <div class="sb-logo">
      <span class="sb-logo-ai">AI</span>
      <svg class="sb-logo-mark" width="18" height="18" viewBox="0 0 20 20" aria-hidden="true">
        <defs>
          <linearGradient id="sbLogoMark" x1="0" y1="0" x2="1" y2="1">
            <stop offset="0" stop-color="#5ab6ff" />
            <stop offset="1" stop-color="#2f6bff" />
          </linearGradient>
        </defs>
        <path d="M10 1.5C10 6 10 6 14.2 8 10 10 10 10 10 18.5 10 10 10 10 5.8 8 10 6 10 6 10 1.5Z" fill="url(#sbLogoMark)" />
      </svg>
      <span class="sb-logo-hot">Cool</span>
      <small>2.0</small>
    </div>

    <nav class="sb-nav" aria-label="主导航">
      <div class="sb-nav-group">
        <div class="sb-group">内容</div>
        <a class="sb-item" href="/"><span class="sb-icon" aria-hidden="true">▤</span><span>AI 日报</span></a>
        <a class="sb-item active" href="/#all"><span class="sb-icon" aria-hidden="true">≣</span><span>AI 动态</span></a>
        <a class="sb-item" href="/#graph"><span class="sb-icon" aria-hidden="true">◇</span><span>话题图谱</span></a>
      </div>
      <div class="sb-nav-group">
        <div class="sb-group">工具</div>
        <a class="sb-item" href="/#tools-discovered"><span class="sb-icon" aria-hidden="true">◎</span><span>工具百宝箱</span><span class="sb-badge" data-sidebar-count="discovered" hidden aria-hidden="true"></span></a>
        <a class="sb-item" href="/#tools-team"><span class="sb-icon" aria-hidden="true">⌁</span><span>团队工具</span><span class="sb-badge" data-sidebar-count="team" hidden aria-hidden="true"></span></a>
      </div>
      <div class="sb-nav-group">
        <div class="sb-group">设置</div>
        <a class="sb-item" href="/#tools-configs"><span class="sb-icon" aria-hidden="true">☷</span><span>发现配置</span><span class="sb-badge" data-sidebar-count="configs" hidden aria-hidden="true"></span></a>
      </div>
    </nav>

    <div class="sb-foot">
      <strong>AI 小组</strong>
      <span>内部最佳实践</span>
    </div>
  </aside>
  <main class="main">
    <article class="article">
      <div class="topbar">
        <a class="back" href="/" onclick="return __goBack()">← 返回</a>
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
      <div class="orig-h">原文{{if .HasTranslation}} <button type="button" class="tr-toggle" id="tr-toggle" onclick="__toggleOrig()">看英文原文</button>{{end}}{{if .CanRetranslate}} <button type="button" class="tr-toggle tr-retry-btn" onclick="__retranslate('{{.ID}}')">{{if .HasTranslation}}重新翻译{{else}}翻译{{end}}</button>{{end}}</div>
      <div class="orig" id="orig-cn">{{.Body}}</div>
      {{if .HasTranslation}}<div class="orig" id="orig-en" style="display:none">{{.BodyOriginal}}</div>
      <p class="tr-note">本文由 DeepSeek 翻译{{if .CanRetranslate}} · <button type="button" class="tr-relink tr-retry-btn" onclick="__retranslate('{{.ID}}')">重新翻译</button>{{end}}</p>{{end}}
      {{end}}
      <div class="tags">
        {{if .Category}}<span class="tag">#{{.Category}}</span>{{end}}
      </div>
      <a class="readmore" href="{{.URL}}" target="_blank" rel="noopener nofollow">阅读原文 →</a>
      <p class="note">本页为站内中文呈现；正文版权归原信源所有，请以「阅读原文」为准。</p>
    </article>
  </main>
</div>
</body>
</html>`))

var notFoundTemplate = template.Must(template.New("404").Parse(`<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><meta name="robots" content="noindex"><title>未找到 · AI Cool</title>
<style>body{max-width:600px;margin:0 auto;padding:48px 18px;font:16px/1.6 system-ui,sans-serif;text-align:center;color-scheme:light dark;}a{color:#2f6bff;}</style>
</head>
<body>
<h1>未找到该资讯</h1>
<p>这条内容可能已下线或不存在。</p>
<a href="/">← 返回 AI Cool</a>
</body>
</html>`))
