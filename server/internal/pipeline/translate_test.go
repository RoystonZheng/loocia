package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func fakeReturning(s string) *fakeLLM { return &fakeLLM{reply: s} }

func TestTranslatePassesThroughModelOutput(t *testing.T) {
	f := &fakeLLM{reply: "<p>这是中文正文。</p>"}
	tr := NewTranslator(f, "test-model")
	got, err := tr.Translate(context.Background(), "<p>English body.</p>")
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if got != "<p>这是中文正文。</p>" {
		t.Fatalf("translated: %q", got)
	}
	if !strings.Contains(f.gotUser, "English body") {
		t.Fatalf("body not sent to model: %q", f.gotUser)
	}
}

func TestTranslatePropagatesError(t *testing.T) {
	f := &fakeLLM{err: errors.New("boom")}
	tr := NewTranslator(f, "test-model")
	if _, err := tr.Translate(context.Background(), "x"); err == nil {
		t.Fatal("expected error")
	}
}

func TestTranslateSystemPromptKeepsTagsAndTerms(t *testing.T) {
	for _, kw := range []string{"HTML", "标签", "术语"} {
		if !strings.Contains(translateSystemPrompt, kw) {
			t.Fatalf("translate prompt missing %q", kw)
		}
	}
}

func TestLooksUntranslated(t *testing.T) {
	src := "<p>OpenAI released a new model today with major improvements.</p>"
	bad := []string{
		"请提供需要翻译的 HTML 正文内容，我会按要求进行处理。",                                     // refusal
		"<p>OpenAI released a new model today with major improvements.</p>", // echoed English
		"As an AI, I cannot translate this.",                                // english meta-refusal
		"   ",                                                               // empty
	}
	for _, b := range bad {
		if !looksUntranslated(src, b) {
			t.Errorf("should flag as untranslated: %q", b)
		}
	}
	good := []string{
		"<p>OpenAI 今天发布了一款有重大改进的新模型。</p>",
		"<p>这是一段包含少量 English 术语（如 Transformer）的正常中文译文。</p>",
	}
	for _, g := range good {
		if looksUntranslated(src, g) {
			t.Errorf("should accept valid Chinese: %q", g)
		}
	}
}

func TestTranslateRejectsBadOutput(t *testing.T) {
	// fake returns echoed English -> Translate must return errUntranslated, not the garbage.
	f := fakeReturning("<p>English body unchanged here for sure.</p>")
	tr := NewTranslator(f, "deepseek-v4-flash")
	_, err := tr.Translate(context.Background(), "<p>English body unchanged here for sure.</p>")
	if err == nil {
		t.Fatal("expected error on untranslated output")
	}
}

func TestTranslatePassesGoodOutput(t *testing.T) {
	f := fakeReturning("<p>正常的中文译文内容。</p>")
	tr := NewTranslator(f, "deepseek-v4-flash")
	out, err := tr.Translate(context.Background(), "<p>English.</p>")
	if err != nil || out != "<p>正常的中文译文内容。</p>" {
		t.Fatalf("good output should pass through: %q %v", out, err)
	}
}

func TestTranslatorModel(t *testing.T) {
	tr := NewTranslator(fakeReturning("x"), "deepseek-v4-flash")
	if tr.Model() != "deepseek-v4-flash" {
		t.Fatalf("Model(): %q", tr.Model())
	}
}

func TestTranslateCapsLongBody(t *testing.T) {
	f := &fakeLLM{reply: "ok"}
	tr := NewTranslator(f, "test-model")
	body := strings.Repeat("word ", 5000)
	if _, err := tr.Translate(context.Background(), body); err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if n := len([]rune(f.gotUser)); n > maxTranslateRunes+50 {
		t.Fatalf("body not capped: %d runes", n)
	}
}
