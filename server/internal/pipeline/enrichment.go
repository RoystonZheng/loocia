package pipeline

import (
	"encoding/json"
	"fmt"
	"strings"

	"aihot-server/internal/ingest"
	"aihot-server/internal/items"
)

// Enrichment is the structured LLM output for one item.
type Enrichment struct {
	TitleCN   string `json:"title_cn"`
	SummaryCN string `json:"summary_cn"`
	Category  string `json:"category"`
	Relevance int    `json:"relevance"`
	Score     int    `json:"score"`
	Selected  bool   `json:"selected"`
}

var validCategories = map[string]bool{
	items.CategoryAIModels:   true,
	items.CategoryAIProducts: true,
	items.CategoryIndustry:   true,
	items.CategoryPaper:      true,
	items.CategoryTip:        true,
}

const enrichSystemPrompt = `你是 AI 资讯编辑。给定一条资讯的原始标题和正文，输出一个 JSON 对象（只输出 JSON，不要任何解释或代码块外的文字），字段：
- "title_cn": 简洁的中文标题
- "summary_cn": 2-3 句中文摘要
- "category": 必须是以下之一：ai-models、ai-products、industry、paper、tip
- "relevance": 整数 0-100，该资讯与「AI/大模型」主题的相关度
- "score": 整数 0-100，值得阅读的程度
- "selected": 布尔，是否值得进入每日精选`

func buildPrompt(r ingest.RawItem) (system, user string) {
	var b strings.Builder
	b.WriteString("原始标题：")
	b.WriteString(r.Title)
	if r.RawContent != nil && *r.RawContent != "" {
		b.WriteString("\n\n正文：\n")
		b.WriteString(*r.RawContent)
	}
	return enrichSystemPrompt, b.String()
}

// parseEnrichment extracts and validates the JSON object from the model output.
func parseEnrichment(text string) (Enrichment, error) {
	var e Enrichment
	jsonStr, err := extractJSONObject(text)
	if err != nil {
		return e, err
	}
	if err := json.Unmarshal([]byte(jsonStr), &e); err != nil {
		return e, fmt.Errorf("enrichment json: %w", err)
	}
	if !validCategories[e.Category] {
		return e, fmt.Errorf("invalid category %q", e.Category)
	}
	if e.Relevance < 0 || e.Relevance > 100 || e.Score < 0 || e.Score > 100 {
		return e, fmt.Errorf("score/relevance out of range: rel=%d score=%d", e.Relevance, e.Score)
	}
	if e.TitleCN == "" {
		return e, fmt.Errorf("empty title_cn")
	}
	return e, nil
}

// extractJSONObject strips code fences and returns the outermost {...} span.
func extractJSONObject(text string) (string, error) {
	s := strings.TrimSpace(text)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < 0 || end < start {
		return "", fmt.Errorf("no JSON object found in model output")
	}
	return s[start : end+1], nil
}
