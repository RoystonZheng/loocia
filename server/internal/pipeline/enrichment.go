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
	ReasonCN  string `json:"reason_cn"`
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
- "score": 整数 0-100，值得阅读的程度，按档位打分：
    90-100 = 重大突破/必读：头部实验室的重大模型发布、范式级技术进展、明显改变行业格局的事件；
    75-89  = 高价值：重要的产品或模型更新、有深度的工程实践或研究、值得关注的行业动态；
    60-74  = 值得一看：常规但相关的进展，有一定信息量，无突破性；
    40-59  = 一般：边缘相关、增量很小、偏营销或宣发；
    0-39   = 低：与 AI 关系弱、旧闻炒冷饭、纯公关软文或活动/直播通知。
    打分看实质内容而非标题措辞，不要因为标题吹得响就给高分。
- "selected": 布尔，是否值得进入每日精选
- "reason_cn": 一句话（不超过40字）说明这条为什么值得精选/关注（点出关键看点，不要复述标题）

重要：title_cn、summary_cn、reason_cn 的文本内容里绝对不要出现英文双引号 " ；需要引用词语或名称时，一律改用中文引号「」或书名号《》。字符串值内出现未转义的英文双引号会破坏 JSON。`

// maxPromptBodyRunes caps how much article body we feed the enricher. Full-text
// sources (公众号 articles run past 100k chars) would otherwise blow up the LLM
// request; the title plus the lede/intro is enough to produce a headline,
// summary, category, and relevance score. RSS snippets are far shorter and pass
// through untouched.
const maxPromptBodyRunes = 4000

func buildPrompt(r ingest.RawItem) (system, user string) {
	var b strings.Builder
	b.WriteString("原始标题：")
	b.WriteString(r.Title)
	if r.RawContent != nil && *r.RawContent != "" {
		b.WriteString("\n\n正文：\n")
		b.WriteString(truncateRunes(*r.RawContent, maxPromptBodyRunes))
	}
	return enrichSystemPrompt, b.String()
}

// truncateRunes returns s capped at n runes (with an ellipsis when cut), safe
// for multi-byte (Chinese) text.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
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
