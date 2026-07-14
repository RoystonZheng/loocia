package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestTranslatePassesThroughModelOutput(t *testing.T) {
	f := &fakeLLM{reply: "<p>这是中文正文。</p>"}
	tr := NewTranslator(f)
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
	tr := NewTranslator(f)
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

func TestTranslateCapsLongBody(t *testing.T) {
	f := &fakeLLM{reply: "ok"}
	tr := NewTranslator(f)
	body := strings.Repeat("word ", 5000)
	if _, err := tr.Translate(context.Background(), body); err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if n := len([]rune(f.gotUser)); n > maxTranslateRunes+50 {
		t.Fatalf("body not capped: %d runes", n)
	}
}
