package detailpage

import (
	"strings"
	"testing"
)

func TestRenderBodyMarkdown(t *testing.T) {
	out := string(renderBody("mp", "# 标题\n\n这是**正文**段落。\n\n第二段。"))
	if !strings.Contains(out, "<h1") || !strings.Contains(out, "<p>") {
		t.Fatalf("markdown not rendered to HTML: %q", out)
	}
	if !strings.Contains(out, "<strong>") {
		t.Fatalf("bold not rendered: %q", out)
	}
}

func TestRenderBodyGFMTable(t *testing.T) {
	md := "| 资产类型 | 定义 |\n| --- | --- |\n| 业务规则库 | 是什么怎么算 |\n| SQL 规范库 | 怎么写才合格 |"
	out := string(renderBody("mp", md))
	if !strings.Contains(out, "<table") || !strings.Contains(out, "<td>") {
		t.Fatalf("GFM table not rendered (extension off or sanitized away): %q", out)
	}
	if !strings.Contains(out, "业务规则库") || !strings.Contains(out, "SQL 规范库") {
		t.Fatalf("table cell content missing: %q", out)
	}
	// pipes must NOT survive as literal text
	if strings.Contains(out, "| --- |") {
		t.Fatalf("table delimiter leaked as raw text: %q", out)
	}
}

func TestRenderBodyStripsScripts(t *testing.T) {
	out := string(renderBody("rss", `<p>hi</p><script>alert(1)</script><p onclick="x()">bye</p>`))
	if strings.Contains(out, "<script") || strings.Contains(out, "alert") {
		t.Fatalf("script must be stripped: %q", out)
	}
	if strings.Contains(out, "onclick") {
		t.Fatalf("event handler must be stripped: %q", out)
	}
	if !strings.Contains(out, "hi") || !strings.Contains(out, "bye") {
		t.Fatalf("safe text should survive: %q", out)
	}
}

func TestRenderBodyImagesLazyAndSelfHiding(t *testing.T) {
	out := string(renderBody("mp", "![图](https://cdn.ex.com/a.jpg)"))
	if !strings.Contains(out, "<img") {
		t.Fatalf("image should render: %q", out)
	}
	if !strings.Contains(out, `loading="lazy"`) || !strings.Contains(out, "onerror=") {
		t.Fatalf("image should be lazy + self-hiding: %q", out)
	}
	// no-referrer bypasses WeChat's anti-hotlink placeholder.
	if !strings.Contains(out, `referrerpolicy="no-referrer"`) {
		t.Fatalf("image should send no referrer: %q", out)
	}
}

func TestRenderBodyEmpty(t *testing.T) {
	if renderBody("mp", "   ") != "" {
		t.Fatal("blank body should render empty")
	}
}
