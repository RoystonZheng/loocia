package pipeline

import (
	"context"
	"errors"
	"strings"
	"unicode"
)

// Translator turns an English (HTML) article body into Chinese, preserving tags.
type Translator interface {
	Translate(ctx context.Context, body string) (string, error)
	Model() string
}

const translateSystemPrompt = `你是专业的科技翻译。把用户给的 HTML 正文翻成简体中文，要求：
1. 保留所有 HTML 标签、图片、链接原样不动，只翻译标签之间的文字内容；
2. 技术术语和产品名（如 Transformer、OpenAI、GPU、embedding）按惯例保留英文，不要硬译；
3. 说人话、通顺自然，不要逐字直译；
4. 只输出翻译后的 HTML，不要任何解释、前后缀或代码块标记；
5. 必须输出中文译文；即使正文很短也要翻译；若正文本身已是中文则原样返回；
6. 不要索要内容、不要解释、不要返回英文原文、不要输出除译文外的任何文字。`

// maxTranslateRunes caps the body sent to the model. RSS bodies are short
// (median ~165 chars); the rare long one is truncated to stay under token limits.
const maxTranslateRunes = 8000

type translator struct {
	llm   LLM
	model string
}

// NewTranslator wraps an LLM (typically a lower-tier client) as a Translator.
// model is the model name recorded alongside each translation.
func NewTranslator(llm LLM, model string) Translator { return &translator{llm: llm, model: model} }

func (t *translator) Model() string { return t.model }

func (t *translator) Translate(ctx context.Context, body string) (string, error) {
	out, err := t.llm.Complete(ctx, translateSystemPrompt, truncateRunes(body, maxTranslateRunes))
	if err != nil {
		return "", err
	}
	out = strings.TrimSpace(out)
	if looksUntranslated(body, out) {
		return "", errUntranslated
	}
	return out, nil
}

var errUntranslated = errors.New("translation looks untranslated or refused")

// refusalMarkers are phrases a model emits when it declines or asks for input
// instead of translating. Compared case-insensitively.
var refusalMarkers = []string{
	"请提供", "我会按", "抱歉", "无法翻译", "请将", "作为一个",
	"as an ai", "please provide", "i'll translate", "i cannot translate",
	"i can't translate",
}

// stripTags removes HTML tags so the heuristics see visible text only.
func stripTags(s string) string { return tagRe.ReplaceAllString(s, "") }

// looksUntranslated reports whether out is a bad translation: empty, a refusal /
// meta-reply, or still essentially English (almost no CJK). src is unused for the
// heuristics but kept for signature symmetry / future use.
func looksUntranslated(src, out string) bool {
	trimmed := strings.TrimSpace(stripTags(out))
	if trimmed == "" {
		return true
	}
	low := strings.ToLower(trimmed)
	for _, m := range refusalMarkers {
		if strings.Contains(low, m) {
			return true
		}
	}
	// "still English": enough visible text but almost no Chinese characters.
	if len([]rune(trimmed)) > 20 && cjkRatio(trimmed) < 0.10 {
		return true
	}
	return false
}

func cjkRatio(s string) float64 {
	var cjk, letters int
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Han, r):
			cjk++
			letters++
		case unicode.IsLetter(r):
			letters++
		}
	}
	if letters == 0 {
		return 0
	}
	return float64(cjk) / float64(letters)
}
