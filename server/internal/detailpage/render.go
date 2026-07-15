package detailpage

import (
	"bytes"
	"html/template"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var (
	// GFM adds tables, strikethrough, task lists and autolinks — 公众号 markdown
	// bodies routinely contain tables, which plain goldmark renders as raw pipes.
	mdRenderer = goldmark.New(goldmark.WithExtensions(extension.GFM))
	sanitizer  = bluemonday.UGCPolicy()
	imgTagRe   = regexp.MustCompile(`(?i)<img\s+`)
	// Some 公众号 tables ship an all-empty header row (used as a spacer); GFM
	// still emits a <thead>, which renders as an ugly blank band. Drop a thead
	// whose cells are all empty so the table starts at its first real row.
	emptyTheadRe = regexp.MustCompile(`(?is)<thead>\s*<tr>\s*(?:<th[^>]*>\s*</th>\s*)+</tr>\s*</thead>`)
	// weixinFooterMarkers denote where a WeChat article's export chrome begins —
	// the "preview" hint, permission dialogs, QR code, share bar. Everything from
	// the earliest marker on is boilerplate scraped into the body, not content.
	weixinFooterMarkers = []string{"预览时标签不可点", "微信扫一扫可打开此内容", "轻点两下取消赞"}
)

// stripWeixinFooter truncates an mp body at the first WeChat chrome marker.
func stripWeixinFooter(body string) string {
	cut := len(body)
	for _, m := range weixinFooterMarkers {
		if i := strings.Index(body, m); i >= 0 && i < cut {
			cut = i
		}
	}
	return strings.TrimRight(body[:cut], " \n\t")
}

// renderBody turns an item's stored body into safe HTML for the detail page.
// mp bodies are Markdown (goldmark → HTML); rss bodies are already HTML/text.
// Both are sanitized (bluemonday) before trusting — the body is third-party, so
// this is an XSS guard even though the page is noindex/internal. Images are made
// lazy and self-hiding on error (weixin images are hotlink-protected).
func renderBody(sourceKind, body string) template.HTML {
	if strings.TrimSpace(body) == "" {
		return ""
	}
	var raw string
	if sourceKind == "mp" {
		body = stripWeixinFooter(body)
		var buf bytes.Buffer
		if err := mdRenderer.Convert([]byte(body), &buf); err != nil {
			raw = "<p>" + template.HTMLEscapeString(body) + "</p>"
		} else {
			raw = buf.String()
		}
	} else {
		raw = body
	}
	clean := sanitizer.Sanitize(raw)
	// Balance before the attr injections below: bodies are third-party and LLM
	// translations routinely drop closing tags; an unclosed div here swallows the
	// page's later sibling containers (seen live: #orig-en vanished inside
	// #orig-cn). A parse→render round trip closes everything it opens.
	clean = balanceHTML(clean)
	// Post-sanitize (so these fixed handlers survive): lazy-load, hide broken, and
	// send no Referer — WeChat's mmbiz.qpic.cn serves an anti-hotlink placeholder
	// when the Referer isn't a weixin domain; no-referrer gets the real image.
	clean = imgTagRe.ReplaceAllString(clean, `<img loading="lazy" referrerpolicy="no-referrer" onerror="this.style.display='none'" `)
	clean = emptyTheadRe.ReplaceAllString(clean, "")
	return template.HTML(clean)
}

// balanceHTML re-serializes a fragment through a real HTML parser so every
// opened tag is closed. Parse errors fall back to the input unchanged.
func balanceHTML(fragment string) string {
	ctx := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), ctx)
	if err != nil {
		return fragment
	}
	var buf bytes.Buffer
	for _, n := range nodes {
		if err := html.Render(&buf, n); err != nil {
			return fragment
		}
	}
	return buf.String()
}
