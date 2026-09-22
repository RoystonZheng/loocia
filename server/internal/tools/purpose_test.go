package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestInferPurposeTagsUsesScenarioLabelsAndDoesNotForce(t *testing.T) {
	repo := GitHubRepo{
		FullName:    "microsoft/playwright-mcp",
		Name:        "playwright-mcp",
		Description: "MCP server for browser automation and Playwright testing",
		Topics:      []string{"mcp", "browser-automation", "testing"},
	}
	tags := InferPurposeTags(repo)
	if len(tags) == 0 || tags[0] != "浏览器操作" {
		t.Fatalf("expected browser scenario first, got %+v", tags)
	}
	if !containsTag(tags, "MCP 集成") {
		t.Fatalf("expected MCP tag, got %+v", tags)
	}

	unknown := InferPurposeTags(GitHubRepo{FullName: "example/plain-repo", Description: "tiny utility"})
	if len(unknown) != 0 {
		t.Fatalf("unclear repository should not be forced into a category: %+v", unknown)
	}
}

func TestNormalizePurposeTagsDedupesAndBounds(t *testing.T) {
	tags := NormalizePurposeTags([]string{
		" 浏览器操作 ",
		"浏览器操作",
		"工作流  自动化",
		"这个分类名称非常非常非常非常非常非常长",
	})
	if len(tags) != 3 {
		t.Fatalf("normalized tags mismatch: %+v", tags)
	}
	if tags[0] != "浏览器操作" || tags[1] != "工作流 自动化" {
		t.Fatalf("unexpected normalized order: %+v", tags)
	}
}

func TestLLMPurposeClassifierParsesTagsAndAllowsNewScenario(t *testing.T) {
	classifier := NewLLMPurposeClassifier(fakePurposeLLM{out: "```json\n{\"purpose_tags\":[\"会议纪要\",\"深度研究\"]}\n```"})
	tags, err := classifier.ClassifyPurposeTags(context.Background(), GitHubRepo{
		FullName:    "meeting-labs/agent-notes",
		Description: "summarize meeting transcripts for agents",
	}, DefaultPurposeTags)
	if err != nil {
		t.Fatalf("ClassifyPurposeTags: %v", err)
	}
	if len(tags) != 2 || tags[0] != "会议纪要" || tags[1] != "深度研究" {
		t.Fatalf("unexpected LLM tags: %+v", tags)
	}
}

func TestDiscovererPurposeClassifierFallbackAndNoForce(t *testing.T) {
	repo := GitHubRepo{
		FullName:    "microsoft/playwright-mcp",
		Name:        "playwright-mcp",
		Description: "MCP server for browser automation and Playwright testing",
		Topics:      []string{"mcp", "browser-automation", "testing"},
	}
	d := NewDiscoverer(nil, nil)
	d.PurposeClassifier = fakePurposeClassifier{tags: []string{}}
	if got := d.classifyPurposeTags(context.Background(), repo); len(got) != 0 {
		t.Fatalf("empty AI result should keep repository unclassified, got %+v", got)
	}

	d.PurposeClassifier = fakePurposeClassifier{err: errors.New("llm down")}
	got := d.classifyPurposeTags(context.Background(), repo)
	if !containsTag(got, "浏览器操作") {
		t.Fatalf("LLM failure should fall back to rules, got %+v", got)
	}
}

func TestNormalizedPurposeTagsForStorageUsesAnEmptyArrayWhenUnclassified(t *testing.T) {
	tags := normalizedPurposeTagsForStorage(GitHubRepo{
		FullName:    "example/plain-repo",
		Description: "tiny utility",
	})
	raw, err := json.Marshal(tags)
	if err != nil {
		t.Fatalf("marshal purpose tags: %v", err)
	}
	if string(raw) != "[]" {
		t.Fatalf("unclassified purpose tags = %s, want []", raw)
	}
}

type fakePurposeLLM struct {
	out string
	err error
}

func (f fakePurposeLLM) Complete(context.Context, string, string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.out, nil
}

type fakePurposeClassifier struct {
	tags []string
	err  error
}

func (f fakePurposeClassifier) ClassifyPurposeTags(context.Context, GitHubRepo, []string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.tags, nil
}

func containsTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}
