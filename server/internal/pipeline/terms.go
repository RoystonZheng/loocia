package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aihot-server/internal/terms"
)

// Terms is the structured term-extraction output for one item.
type Terms struct {
	Entities []string `json:"entities"`
	Topics   []string `json:"topics"`
}

// TermExtractor pulls entities + topic keywords out of an item's title+summary.
type TermExtractor interface {
	Extract(ctx context.Context, title, summary string) (Terms, error)
}

const termsSystemPrompt = `你是 AI 资讯的信息抽取器。给定一条资讯的标题和摘要，输出一个 JSON 对象（只输出 JSON，不要任何解释或代码块外的文字），字段：
- "entities": 数组，最多 5 个，资讯里出现的具体实体（公司/组织、模型/产品、人物）。命名要规范统一，同一对象永远用同一个写法：模型与产品保留官方英文名（如 GPT-5.5、Claude、Gemini、DeepSeek-V4、Sora）；公司/组织按下面的规范名写——中文名已通用的用中文：谷歌、微软、英伟达、苹果、亚马逊、阿里、腾讯、字节跳动、百度、华为、智谱、月之暗面；习惯用英文的保留英文：OpenAI、Anthropic、Meta、DeepSeek、Mistral、xAI、Hugging Face；人物用全名（如 Sam Altman、黄仁勋）。
- "topics": 数组，最多 3 个，抽象话题词，每个 2~6 个字的中文（如 推理模型、开源、具身智能、视频生成、芯片、评测）。宁缺毋滥，不要生造。
没有可抽的就给空数组。`

const (
	maxEntities  = 5
	maxTopics    = 3
	maxTermRunes = 40
)

type termExtractor struct{ llm LLM }

// NewTermExtractor wraps an LLM (typically the low-tier terms client).
func NewTermExtractor(llm LLM) TermExtractor { return &termExtractor{llm: llm} }

func (t *termExtractor) Extract(ctx context.Context, title, summary string) (Terms, error) {
	user := "标题：" + title + "\n摘要：" + summary
	out, err := t.llm.Complete(ctx, termsSystemPrompt, user)
	if err != nil {
		return Terms{}, err
	}
	return parseTerms(out)
}

// parseTerms extracts the JSON object and sanitizes both lists: trim, drop
// empties/overlong/duplicates, cap counts. Model quirks are cleaned, not fatal.
func parseTerms(text string) (Terms, error) {
	jsonStr, err := extractJSONObject(text)
	if err != nil {
		return Terms{}, err
	}
	var raw Terms
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return Terms{}, fmt.Errorf("terms json: %w", err)
	}
	return Terms{
		Entities: sanitizeTerms(raw.Entities, maxEntities),
		Topics:   sanitizeTerms(raw.Topics, maxTopics),
	}, nil
}

func sanitizeTerms(in []string, max int) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, max)
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || len([]rune(s)) > maxTermRunes || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
		if len(out) == max {
			break
		}
	}
	return out
}

// termRows flattens extraction output into store rows; a term tagged as both
// entity and topic keeps the entity kind.
func termRows(ts Terms) []terms.Term {
	seen := make(map[string]bool, len(ts.Entities)+len(ts.Topics))
	out := make([]terms.Term, 0, len(ts.Entities)+len(ts.Topics))
	for _, e := range ts.Entities {
		if !seen[e] {
			seen[e] = true
			out = append(out, terms.Term{Term: e, Kind: "entity"})
		}
	}
	for _, t := range ts.Topics {
		if !seen[t] {
			seen[t] = true
			out = append(out, terms.Term{Term: t, Kind: "topic"})
		}
	}
	return out
}

// TermRowsForBackfill exposes the Terms→rows flattening for the backfill cmd.
func TermRowsForBackfill(ts Terms) []terms.Term { return termRows(ts) }
