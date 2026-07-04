package daily

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"aihot-server/internal/items"
)

// LLM is the minimal completion surface (satisfied by *llm.Client).
type LLM interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// buildSections groups items by category in the fixed order, score-desc
// (nil score last), publishedAt-desc tiebreak. Uncategorized items are skipped.
// Empty categories produce no section.
func buildSections(its []items.Item) []Section {
	byCat := map[string][]items.Item{}
	for _, it := range its {
		if it.Category == nil {
			continue
		}
		byCat[*it.Category] = append(byCat[*it.Category], it)
	}
	var out []Section
	for _, cat := range categoryOrder {
		group := byCat[cat]
		if len(group) == 0 {
			continue
		}
		sort.SliceStable(group, func(i, j int) bool {
			si, sj := -1, -1
			if group[i].Score != nil {
				si = *group[i].Score
			}
			if group[j].Score != nil {
				sj = *group[j].Score
			}
			if si != sj {
				return si > sj
			}
			return group[i].SortKey().After(group[j].SortKey())
		})
		sec := Section{Label: categoryLabels[cat]}
		for _, it := range group {
			sec.Items = append(sec.Items, toSectionItem(it))
		}
		out = append(out, sec)
	}
	return out
}

func toSectionItem(it items.Item) SectionItem {
	summary := ""
	if it.Summary != nil {
		summary = *it.Summary
	}
	perma := it.Permalink
	return SectionItem{
		Title:      it.Title,
		Summary:    summary,
		SourceURL:  it.URL,
		SourceName: it.Source,
		Permalink:  &perma,
	}
}

// buildFlashes returns the n most recent items (publishedAt desc) as 快讯.
func buildFlashes(its []items.Item, n int) []Flash {
	sorted := make([]items.Item, len(its))
	copy(sorted, its)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].SortKey().After(sorted[j].SortKey())
	})
	if len(sorted) > n {
		sorted = sorted[:n]
	}
	var out []Flash
	for _, it := range sorted {
		perma := it.Permalink
		out = append(out, Flash{
			Title:       it.Title,
			SourceName:  it.Source,
			SourceURL:   it.URL,
			PublishedAt: it.PublishedAt,
			Permalink:   &perma,
		})
	}
	return out
}

const leadSystemPrompt = `你是 AI 资讯日报主编。给定今天日报的分组条目清单，写一个导语。输出一个 JSON 对象（只输出 JSON），字段：
- "title": 简短有信息量的日报标题（中文）
- "lead_paragraph": 一段 2-4 句的中文导语，概括今天最值得关注的动向`

// generateLead asks the LLM for the day's lead over the built sections.
func generateLead(ctx context.Context, l LLM, secs []Section) (*Lead, error) {
	var b strings.Builder
	for _, s := range secs {
		b.WriteString("## " + s.Label + "\n")
		for _, it := range s.Items {
			b.WriteString("- " + it.Title)
			if it.Summary != "" {
				b.WriteString("：" + it.Summary)
			}
			b.WriteString("\n")
		}
	}
	out, err := l.Complete(ctx, leadSystemPrompt, b.String())
	if err != nil {
		return nil, err
	}
	return parseLead(out)
}

type leadPayload struct {
	Title         string `json:"title"`
	LeadParagraph string `json:"lead_paragraph"`
}

// parseLead extracts and validates the lead JSON (lenient about code fences).
func parseLead(text string) (*Lead, error) {
	s := strings.TrimSpace(text)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < start {
		return nil, fmt.Errorf("no JSON object in lead output")
	}
	var p leadPayload
	if err := json.Unmarshal([]byte(s[start:end+1]), &p); err != nil {
		return nil, fmt.Errorf("lead json: %w", err)
	}
	if p.Title == "" {
		return nil, fmt.Errorf("empty lead title")
	}
	return &Lead{Title: p.Title, LeadParagraph: p.LeadParagraph}, nil
}
