package pipeline

import (
	"context"
	"strings"
)

// Translator turns an English (HTML) article body into Chinese, preserving tags.
type Translator interface {
	Translate(ctx context.Context, body string) (string, error)
}

const translateSystemPrompt = `你是专业的科技翻译。把用户给的 HTML 正文翻成简体中文，要求：
1. 保留所有 HTML 标签、图片、链接原样不动，只翻译标签之间的文字内容；
2. 技术术语和产品名（如 Transformer、OpenAI、GPU、embedding）按惯例保留英文，不要硬译；
3. 说人话、通顺自然，不要逐字直译；
4. 只输出翻译后的 HTML，不要任何解释、前后缀或代码块标记。`

// maxTranslateRunes caps the body sent to the model. RSS bodies are short
// (median ~165 chars); the rare long one is truncated to stay under token limits.
const maxTranslateRunes = 8000

type translator struct{ llm LLM }

// NewTranslator wraps an LLM (typically a lower-tier client) as a Translator.
func NewTranslator(llm LLM) Translator { return &translator{llm: llm} }

func (t *translator) Translate(ctx context.Context, body string) (string, error) {
	out, err := t.llm.Complete(ctx, translateSystemPrompt, truncateRunes(body, maxTranslateRunes))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
