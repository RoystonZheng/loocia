package publicapi

import (
	"strings"
	"testing"

	"aihot-server/internal/tools"
)

func TestFallbackToolSummaryIsChineseAndSpecific(t *testing.T) {
	summary := fallbackToolSummary(tools.Tool{
		Name:           "Agent Browser",
		GitHubFullName: "vercel-labs/agent-browser",
		Description:    strPtrForSummaryTest("A browser automation tool for AI agents"),
		Topics:         []string{"browser-automation", "ai-agents"},
	})
	if !hasHan(summary) {
		t.Fatalf("fallback summary should be Chinese: %q", summary)
	}
	if !strings.Contains(summary, "Agent Browser") || !strings.Contains(summary, "浏览器自动化") {
		t.Fatalf("fallback summary should keep tool name and inferred focus: %q", summary)
	}
}

func strPtrForSummaryTest(v string) *string {
	return &v
}
