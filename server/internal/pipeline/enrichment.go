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
- "relevance": 整数 1-5，与「AI/大模型」主题的相关度：5=核心 AI、3=沾边、1=几乎无关
- "score": 整数 1-5，值得阅读的程度。判分内核 = （工程可操作 或 行业重要）×（深度/证据）：
    工程可操作 = 有方法/代码/benchmark/踩坑，能照着用或学；
    行业重要 = 头部玩家、格局变化、战略或投资信号、能力边界；
    深度/证据 = 讲清机制、有实测数据（浅的工程贴和浅的行业吹都要压低）。
    档位：
    5 = 重大突破/必读：一条腿打满且深度证据强（头部重大发布并讲透机制或有硬数据，或范式级工程实践带完整方法与 benchmark）；
    4 = 高价值：一条腿强且有实质深度（重要模型/产品更新且有分析，或扎实的工程实践或研究）；
    3 = 值得一看：相关且有信息量，但深度一般或影响有限；
    2 = 一般：边缘、增量很小、偏宣发、浅尝；
    1 = 低：营销通稿、标题党无实质、活动/直播/招聘/预告等通知、与 AI 弱相关。
    规则：工程腿和行业腿平权，两者都能到 5；看实质内容而非标题措辞；
    旧闻/不新颖/人尽皆知只要深且有用不扣分；压低营销宣发通知、标题党、与 AI 弱相关。
- "reason_cn": 一句话（不超过40字）说明这条为什么值得关注（点出关键看点，不要复述标题）

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
	e.Score = clampTier(e.Score)
	e.Relevance = clampTier(e.Relevance)
	if e.TitleCN == "" {
		return e, fmt.Errorf("empty title_cn")
	}
	return e, nil
}

// clampTier bounds a 1-5 ordinal; out-of-range model output is clamped rather
// than rejected, so a stray 0/6/old-scale value never drops the whole item.
func clampTier(n int) int {
	if n < 1 {
		return 1
	}
	if n > 5 {
		return 5
	}
	return n
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
