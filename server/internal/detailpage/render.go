package detailpage

import (
	"bytes"
	"html/template"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
)

var (
	mdRenderer = goldmark.New()
	sanitizer  = bluemonday.UGCPolicy()
	imgTagRe   = regexp.MustCompile(`(?i)<img\s+`)
)

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
	// Post-sanitize (so these fixed handlers survive): lazy-load, hide broken, and
	// send no Referer — WeChat's mmbiz.qpic.cn serves an anti-hotlink placeholder
	// when the Referer isn't a weixin domain; no-referrer gets the real image.
	clean = imgTagRe.ReplaceAllString(clean, `<img loading="lazy" referrerpolicy="no-referrer" onerror="this.style.display='none'" `)
	return template.HTML(clean)
}
