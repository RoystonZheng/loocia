package detailpage

import (
	"strings"

	"aihot-server/internal/items"
)

// buildMarkdown renders an item as a downloadable Markdown document: title,
// a source/date/link blockquote, 精选理由 (when set), 摘要, and 原文. MP bodies
// are already Markdown; RSS HTML is passed through best-effort.
func buildMarkdown(it *items.Item) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(it.Title)
	b.WriteString("\n\n> ")
	b.WriteString(it.Source)
	if it.PublishedAt != nil {
		b.WriteString(" · ")
		b.WriteString(it.PublishedAt.In(beijing).Format("2006-01-02 15:04"))
	}
	b.WriteString(" · ")
	b.WriteString(it.URL)
	b.WriteString("\n")

	if it.Reason != nil && *it.Reason != "" {
		b.WriteString("\n**精选理由**：")
		b.WriteString(*it.Reason)
		b.WriteString("\n")
	}
	if it.Summary != nil && *it.Summary != "" {
		b.WriteString("\n## 摘要\n\n")
		b.WriteString(*it.Summary)
		b.WriteString("\n")
	}
	if it.SourceKind == "rss" && it.BodyCN != nil && strings.TrimSpace(*it.BodyCN) != "" {
		b.WriteString("\n## 原文（AI 翻译）\n\n")
		b.WriteString(*it.BodyCN)
		b.WriteString("\n")
	} else if it.Body != nil && strings.TrimSpace(*it.Body) != "" {
		b.WriteString("\n## 原文\n\n")
		b.WriteString(*it.Body)
		b.WriteString("\n")
	}
	return b.String()
}
