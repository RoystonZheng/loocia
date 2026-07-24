package detailpage

import (
	"strings"
	"testing"

	"aihot-server/internal/items"
)

func TestToViewModelScoreLetterAndTier(t *testing.T) {
	cases := []struct {
		score      int
		wantLetter string
	}{
		{5, "S"}, {4, "A"}, {3, "B"}, {2, "C"}, {1, "D"},
	}
	for _, c := range cases {
		s := c.score
		it := &items.Item{ID: "x", Title: "标题", URL: "https://ex.com/x", Permalink: "/items/x", Source: "S", Score: &s, Selected: true}
		vm := toViewModel(it)
		if !vm.HasScore {
			t.Fatalf("score=%d: HasScore should be true", c.score)
		}
		if vm.Score != c.wantLetter || vm.Tier != c.wantLetter {
			t.Fatalf("score=%d: got Score=%q Tier=%q, want %q", c.score, vm.Score, vm.Tier, c.wantLetter)
		}
	}
}

func TestToViewModelNoScore(t *testing.T) {
	it := &items.Item{ID: "x", Title: "t", URL: "https://ex.com/x", Permalink: "/items/x", Source: "S"}
	vm := toViewModel(it)
	if vm.HasScore || vm.Score != "" || vm.Tier != "" {
		t.Fatalf("no score: HasScore=%v Score=%q Tier=%q", vm.HasScore, vm.Score, vm.Tier)
	}
}

func TestScoreLetterClamps(t *testing.T) {
	if scoreLetter(9) != "S" || scoreLetter(0) != "D" {
		t.Fatalf("clamp: 9=%q 0=%q", scoreLetter(9), scoreLetter(0))
	}
}

func TestToViewModelTranslation(t *testing.T) {
	en := "<p>English.</p>"
	cn := "<p>中文。</p>"
	rss := &items.Item{ID: "x", Title: "t", URL: "https://e.com/x", Permalink: "/items/x",
		Source: "S", SourceKind: "rss", Body: &en, BodyCN: &cn}
	vm := toViewModel(rss)
	if !vm.HasTranslation {
		t.Fatal("rss+body_cn should have translation")
	}
	if !strings.Contains(string(vm.Body), "中文") {
		t.Fatalf("default body should be Chinese: %q", vm.Body)
	}
	if !strings.Contains(string(vm.BodyOriginal), "English") {
		t.Fatalf("original should be English: %q", vm.BodyOriginal)
	}

	mpb := "# 标题\n正文"
	mp := &items.Item{ID: "y", Title: "t", URL: "https://e.com/y", Permalink: "/items/y",
		Source: "S", SourceKind: "mp", Body: &mpb}
	if toViewModel(mp).HasTranslation {
		t.Fatal("mp should not have translation toggle")
	}

	rssNoCN := &items.Item{ID: "z", Title: "t", URL: "https://e.com/z", Permalink: "/items/z",
		Source: "S", SourceKind: "rss", Body: &en}
	vm3 := toViewModel(rssNoCN)
	if vm3.HasTranslation {
		t.Fatal("rss without body_cn should not have toggle")
	}
	if !strings.Contains(string(vm3.Body), "English") {
		t.Fatalf("fallback body should be English original: %q", vm3.Body)
	}
}

// renderDetail builds the view model and renders the full HTML page for an item.
func renderDetail(t *testing.T, it *items.Item) string {
	t.Helper()
	var b strings.Builder
	if err := pageTemplate.Execute(&b, toViewModel(it)); err != nil {
		t.Fatalf("execute: %v", err)
	}
	return b.String()
}

func TestDetailBackLinkAlwaysPresent(t *testing.T) {
	body := "# t\n正文"
	out := renderDetail(t, &items.Item{ID: "x", Title: "t", URL: "https://e.com/x", Permalink: "/items/x",
		Source: "S", SourceKind: "mp", Body: &body})
	for _, want := range []string{`class="back"`, "← 返回", `href="/"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("back link missing %q", want)
		}
	}
}

func TestDetailShowsModelNoteWhenTranslated(t *testing.T) {
	en := "<p>English.</p>"
	cn := "<p>中文。</p>"
	model := "deepseek-v4-flash"
	out := renderDetail(t, &items.Item{ID: "x", Title: "t", URL: "https://e.com/x", Permalink: "/items/x",
		Source: "S", SourceKind: "rss", Body: &en, BodyCN: &cn, BodyCNModel: &model})
	if !strings.Contains(out, "本文由 DeepSeek 翻译") {
		t.Fatalf("DeepSeek attribution missing")
	}
	// 已翻译的文章也应有「重新翻译」按钮(供用户重译不完整/不满意的译文)
	if !strings.Contains(out, `tr-toggle tr-retry-btn`) {
		t.Fatalf("retranslate button should be present when translated")
	}
	if !strings.Contains(out, "重新翻译") {
		t.Fatalf("retranslate label 重新翻译 missing when translated")
	}
}

func TestDetailShowsRetryWhenTranslationMissing(t *testing.T) {
	en := "<p>English.</p>"
	out := renderDetail(t, &items.Item{ID: "abc123", Title: "t", URL: "https://e.com/x", Permalink: "/items/abc123",
		Source: "S", SourceKind: "rss", Body: &en})
	if !strings.Contains(out, `tr-toggle tr-retry-btn`) {
		t.Fatalf("retry button missing")
	}
	if !strings.Contains(out, "__retranslate('abc123')") {
		t.Fatalf("retry button should call __retranslate with item id")
	}
	if strings.Contains(out, "本文由 AI") {
		t.Fatalf("model note should be absent when translation missing")
	}
}

func TestDetailNoRetryForMP(t *testing.T) {
	body := "# t\n正文"
	out := renderDetail(t, &items.Item{ID: "x", Title: "t", URL: "https://e.com/x", Permalink: "/items/x",
		Source: "S", SourceKind: "mp", Body: &body})
	if strings.Contains(out, `tr-toggle tr-retry-btn`) {
		t.Fatalf("mp item should not have retry button")
	}
	if strings.Contains(out, "本文由 AI") {
		t.Fatalf("mp item should not have model note")
	}
	if !strings.Contains(out, `class="back"`) {
		t.Fatalf("mp item should still have back link")
	}
}

func TestPageTemplateHasToggle(t *testing.T) {
	en := "<p>English.</p>"
	cn := "<p>中文。</p>"
	vm := toViewModel(&items.Item{ID: "x", Title: "t", URL: "https://e.com/x", Permalink: "/items/x",
		Source: "S", SourceKind: "rss", Body: &en, BodyCN: &cn})
	var b strings.Builder
	if err := pageTemplate.Execute(&b, vm); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := b.String()
	for _, want := range []string{"看英文原文", "__toggleOrig", "orig-en", "orig-cn"} {
		if !strings.Contains(out, want) {
			t.Fatalf("toggle markup missing %q", want)
		}
	}
}
