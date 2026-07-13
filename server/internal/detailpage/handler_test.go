package detailpage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aihot-server/internal/items"
)

// fakeGetter serves canned items by id.
type fakeGetter struct{ byID map[string]*items.Item }

func (f fakeGetter) GetByID(ctx context.Context, id string) (*items.Item, error) {
	if it, ok := f.byID[id]; ok {
		return it, nil
	}
	return nil, items.ErrNotFound
}

func mkItem(id string) *items.Item {
	pub := time.Date(2026, 5, 7, 4, 0, 0, 0, time.UTC) // 12:00 Beijing
	en := "Original English Title"
	summary := "这是中文摘要。"
	cat := items.CategoryAIModels
	score := 88
	return &items.Item{
		ID: id, Title: "中文标题", TitleEN: &en, URL: "https://source.example/post",
		Permalink: "/items/" + id, Source: "OpenAI Blog", PublishedAt: &pub,
		Summary: &summary, Category: &cat, Score: &score, Selected: true, Present: true,
	}
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	return rr
}

func TestRendersItemPage(t *testing.T) {
	h := NewHandler(fakeGetter{byID: map[string]*items.Item{"abc": mkItem("abc")}})
	rr := get(t, h, "/items/abc")
	if rr.Code != http.StatusOK {
		t.Fatalf("code: %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content-type: %q", ct)
	}
	body := rr.Body.String()
	for _, want := range []string{
		"<!doctype html>", `<meta name="robots" content="noindex"`,
		"中文标题", "这是中文摘要。", "OpenAI Blog", "模型发布/更新",
		"2026-05-07 12:00", "Original English Title",
		`href="https://source.example/post"`, "阅读原文", // outbound
		`href="/"`, "全部 AI 动态", // sidebar shell nav back to the SPA
		"AI 摘要",         // summary box label
		"导出 Markdown",   // export affordance
		"<title>中文标题", // page title
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in page:\n%s", want, body)
		}
	}
}

func TestMissingItemIs404(t *testing.T) {
	h := NewHandler(fakeGetter{byID: map[string]*items.Item{}})
	rr := get(t, h, "/items/nope")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "未找到") {
		t.Fatalf("404 page should be friendly HTML: %s", rr.Body.String())
	}
}

func TestHiddenItemsAre404(t *testing.T) {
	withdrawn := mkItem("w")
	withdrawn.Present = false
	dupID := "canonical"
	duplicate := mkItem("d")
	duplicate.DuplicateOfID = &dupID
	h := NewHandler(fakeGetter{byID: map[string]*items.Item{"w": withdrawn, "d": duplicate}})
	if rr := get(t, h, "/items/w"); rr.Code != http.StatusNotFound {
		t.Fatalf("withdrawn should 404, got %d", rr.Code)
	}
	if rr := get(t, h, "/items/d"); rr.Code != http.StatusNotFound {
		t.Fatalf("duplicate should 404, got %d", rr.Code)
	}
}

func TestEmptyIDIs404(t *testing.T) {
	h := NewHandler(fakeGetter{byID: map[string]*items.Item{}})
	if rr := get(t, h, "/items/"); rr.Code != http.StatusNotFound {
		t.Fatalf("empty id should 404, got %d", rr.Code)
	}
}

func TestHTMLIsEscaped(t *testing.T) {
	evil := mkItem("x")
	evil.Title = `<script>alert(1)</script>`
	h := NewHandler(fakeGetter{byID: map[string]*items.Item{"x": evil}})
	body := get(t, h, "/items/x").Body.String()
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Fatalf("title must be escaped, not raw: %s", body)
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Fatalf("expected escaped title: %s", body)
	}
}

func TestRendersImage(t *testing.T) {
	it := mkItem("img")
	img := "https://cdn.ex.com/hero.jpg"
	it.ImageURL = &img
	h := NewHandler(fakeGetter{byID: map[string]*items.Item{"img": it}})
	body := get(t, h, "/items/img").Body.String()
	if !strings.Contains(body, `<img src="https://cdn.ex.com/hero.jpg"`) {
		t.Fatalf("expected image tag, got:\n%s", body)
	}
}

func TestRendersVideoFile(t *testing.T) {
	it := mkItem("vid")
	vid := "https://cdn.ex.com/clip.mp4"
	poster := "https://cdn.ex.com/hero.jpg"
	it.VideoURL = &vid
	it.ImageURL = &poster
	h := NewHandler(fakeGetter{byID: map[string]*items.Item{"vid": it}})
	body := get(t, h, "/items/vid").Body.String()
	if !strings.Contains(body, `<video controls`) || !strings.Contains(body, `src="https://cdn.ex.com/clip.mp4"`) {
		t.Fatalf("expected video tag, got:\n%s", body)
	}
	if !strings.Contains(body, `poster="https://cdn.ex.com/hero.jpg"`) {
		t.Fatalf("expected poster from image, got:\n%s", body)
	}
	// A direct file must NOT become an iframe.
	if strings.Contains(body, "<iframe") {
		t.Fatalf("direct video file should not render as iframe:\n%s", body)
	}
}

func TestRendersVideoEmbed(t *testing.T) {
	it := mkItem("emb")
	vid := "https://player.ex.com/embed/42"
	it.VideoURL = &vid
	h := NewHandler(fakeGetter{byID: map[string]*items.Item{"emb": it}})
	body := get(t, h, "/items/emb").Body.String()
	if !strings.Contains(body, `<iframe src="https://player.ex.com/embed/42"`) {
		t.Fatalf("expected iframe embed, got:\n%s", body)
	}
}

func TestUsesCoolBlueAccent(t *testing.T) {
	h := NewHandler(fakeGetter{byID: map[string]*items.Item{"abc": mkItem("abc")}})
	body := get(t, h, "/items/abc").Body.String()
	if !strings.Contains(body, "#2f6bff") {
		t.Fatalf("expected cool-blue accent, got:\n%s", body)
	}
	if strings.Contains(body, "#1a7f5a") {
		t.Fatalf("stale green accent still present:\n%s", body)
	}
}

func TestRendersReasonAndBody(t *testing.T) {
	it := mkItem("rb")
	reason := "首个把 Harness 工程化落地的实战复盘"
	body := "# 段落标题\n\n这是**正文**内容。"
	it.Reason = &reason
	it.Body = &body
	it.SourceKind = "mp"
	h := NewHandler(fakeGetter{byID: map[string]*items.Item{"rb": it}})
	out := get(t, h, "/items/rb").Body.String()
	if !strings.Contains(out, "精选理由") || !strings.Contains(out, reason) {
		t.Fatalf("reason box missing:\n%s", out)
	}
	if !strings.Contains(out, "原文") || !strings.Contains(out, "<strong>正文</strong>") {
		t.Fatalf("rendered body missing:\n%s", out)
	}
}

func TestReasonBoxAbsentWhenNil(t *testing.T) {
	it := mkItem("nr") // mkItem sets no Reason
	h := NewHandler(fakeGetter{byID: map[string]*items.Item{"nr": it}})
	if strings.Contains(get(t, h, "/items/nr").Body.String(), "精选理由") {
		t.Fatal("reason box should be absent when Reason is nil")
	}
}

func TestExportMarkdown(t *testing.T) {
	it := mkItem("md")
	body := "# 原始\n\n正文段落。"
	it.Body = &body
	it.SourceKind = "mp"
	h := NewHandler(fakeGetter{byID: map[string]*items.Item{"md": it}})
	rr := get(t, h, "/items/md?format=md")
	if rr.Code != http.StatusOK {
		t.Fatalf("code: %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Fatalf("content-type: %q", ct)
	}
	if cd := rr.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Fatalf("expected download disposition: %q", cd)
	}
	out := rr.Body.String()
	for _, want := range []string{"# 中文标题", "## 摘要", "## 原文", "正文段落。", "https://source.example/post"} {
		if !strings.Contains(out, want) {
			t.Fatalf("markdown missing %q:\n%s", want, out)
		}
	}
	// Markdown export must not be HTML.
	if strings.Contains(out, "<!doctype html>") {
		t.Fatalf("export should be markdown, not HTML:\n%s", out)
	}
}

func TestNullableFieldsOmitted(t *testing.T) {
	minimal := &items.Item{
		ID: "m", Title: "仅标题", URL: "https://s/x", Permalink: "/items/m",
		Source: "S", Present: true,
	}
	h := NewHandler(fakeGetter{byID: map[string]*items.Item{"m": minimal}})
	rr := get(t, h, "/items/m")
	if rr.Code != http.StatusOK {
		t.Fatalf("code: %d", rr.Code)
	}
	body := rr.Body.String()
	if strings.Contains(body, "&lt;nil&gt;") || strings.Contains(body, "<nil>") {
		t.Fatalf("nil pointer leaked into page: %s", body)
	}
	if !strings.Contains(body, "仅标题") {
		t.Fatalf("title should render: %s", body)
	}
}
