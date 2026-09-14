package publicapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"aihot-server/internal/tools"
)

type ToolSummaryInput struct {
	URLKey         string
	GitHubFullName string
	Description    string
	Topics         []string
}

type ToolSummaryGenerator interface {
	GenerateToolSummary(ctx context.Context, input ToolSummaryInput) (string, error)
}

type toolSummaryLLM interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

type llmToolSummaryGenerator struct {
	llm toolSummaryLLM
}

func NewLLMToolSummaryGenerator(llm toolSummaryLLM) ToolSummaryGenerator {
	return llmToolSummaryGenerator{llm: llm}
}

type toolSummaryRequest struct {
	ToolID string `json:"toolId"`
	URLKey string `json:"urlKey"`
}

type toolSummaryResponse struct {
	ToolID     string `json:"toolId"`
	URLKey     string `json:"urlKey"`
	Summary    string `json:"summary"`
	Source     string `json:"source"`
	Translated bool   `json:"translated"`
}

const toolSummaryGenerationTimeout = 300 * time.Millisecond

func (h *ToolAPIHandler) handleToolSummaryByURLKey(w http.ResponseWriter, r *http.Request) {
	if err := requireToolStore(h.store); err != nil {
		writeToolErr(w, err)
		return
	}
	var req toolSummaryRequest
	if err := decodeToolJSON(r, &req); err != nil {
		writeToolErr(w, err)
		return
	}
	toolID, err := requireToolText(req.ToolID, "toolId", 120)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	urlKey, err := requireToolText(req.URLKey, "urlKey", 80)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	tool, err := h.store.GetTool(r.Context(), toolID)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	expectedKey := toolURLKey(tool.GitHubURL)
	if urlKey != expectedKey {
		writeToolErr(w, &tools.DiscoveryError{Class: tools.ErrorValidation, Message: "urlKey does not match tool URL"})
		return
	}
	if summary, source, translated := reusableToolSummary(tool, urlKey); summary != "" {
		writeToolOK(w, toolSummaryResponse{ToolID: tool.ID, URLKey: urlKey, Summary: summary, Source: source, Translated: translated})
		return
	}
	input := ToolSummaryInput{
		URLKey:         urlKey,
		GitHubFullName: tool.GitHubFullName,
		Description:    optionalStringValue(tool.Description),
		Topics:         tool.Topics,
	}
	source := "url_key:" + urlKey
	generated := ""
	if h.summaryGenerator != nil {
		ctx, cancel := context.WithTimeout(r.Context(), toolSummaryGenerationTimeout)
		defer cancel()
		if out, err := h.summaryGenerator.GenerateToolSummary(ctx, input); err == nil {
			generated = cleanToolSummary(out)
		}
	}
	if generated == "" || !hasHan(generated) {
		generated = fallbackToolSummary(tool)
		source = "url_key_fallback:" + urlKey
	}
	if err := h.store.UpdateTemporarySummary(r.Context(), tool.ID, generated, source); err != nil {
		writeToolErr(w, err)
		return
	}
	writeToolOK(w, toolSummaryResponse{ToolID: tool.ID, URLKey: urlKey, Summary: generated, Source: source, Translated: true})
}

func (g llmToolSummaryGenerator) GenerateToolSummary(ctx context.Context, input ToolSummaryInput) (string, error) {
	if g.llm == nil {
		return "", fmt.Errorf("llm is nil")
	}
	system := "你是 AI Cool 工具库编辑。把 GitHub 仓库介绍整理成简体中文，1 句话，最多 80 个中文字符。只输出简介，不要列表、标题、引号或解释。"
	user := fmt.Sprintf("仓库：%s\nURL Key：%s\n英文介绍：%s\nTopics：%s",
		input.GitHubFullName,
		input.URLKey,
		strings.TrimSpace(input.Description),
		strings.Join(input.Topics, ", "),
	)
	return g.llm.Complete(ctx, system, user)
}

func reusableToolSummary(tool tools.Tool, urlKey string) (summary, source string, translated bool) {
	if tool.FinalSummary != nil && strings.TrimSpace(*tool.FinalSummary) != "" {
		return strings.TrimSpace(*tool.FinalSummary), "final", hasHan(*tool.FinalSummary)
	}
	if tool.TemporarySummary != nil && strings.TrimSpace(*tool.TemporarySummary) != "" {
		source := "temporary"
		if tool.TemporarySummarySource != nil && strings.TrimSpace(*tool.TemporarySummarySource) != "" {
			source = strings.TrimSpace(*tool.TemporarySummarySource)
		}
		return strings.TrimSpace(*tool.TemporarySummary), source, strings.Contains(source, "url_key:"+urlKey) || hasHan(*tool.TemporarySummary)
	}
	if tool.Description != nil && strings.TrimSpace(*tool.Description) != "" && hasHan(*tool.Description) {
		return strings.TrimSpace(*tool.Description), "github_description", true
	}
	return "", "", false
}

func fallbackToolSummary(tool tools.Tool) string {
	name := strings.TrimSpace(tool.Name)
	if name == "" {
		name = strings.TrimSpace(tool.GitHubFullName)
	}
	if name == "" {
		name = "该仓库"
	}
	focus := inferToolFocus(tool)
	if focus != "" {
		return cleanToolSummary(fmt.Sprintf("%s 是一个面向 %s 的开源工具，可用于团队测评和选型。", name, focus))
	}
	return cleanToolSummary(fmt.Sprintf("%s 是 GitHub 上的开源工具，可先结合仓库说明评估使用场景。", name))
}

func inferToolFocus(tool tools.Tool) string {
	corpusParts := []string{tool.Name, tool.GitHubFullName, optionalStringValue(tool.Description)}
	corpusParts = append(corpusParts, tool.Topics...)
	corpus := strings.ToLower(strings.Join(corpusParts, " "))
	rules := []struct {
		keys  []string
		label string
	}{
		{[]string{"mcp", "model-context-protocol"}, "MCP 接入和上下文协作"},
		{[]string{"browser", "automation", "computer-use"}, "浏览器自动化"},
		{[]string{"coding", "code", "developer", "review"}, "编码辅助和代码协作"},
		{[]string{"research", "search", "deepsearch"}, "资料检索和研究"},
		{[]string{"agent", "agents", "autonomous"}, "Agent 工作流"},
		{[]string{"rag", "embedding", "vector"}, "知识检索"},
		{[]string{"workflow", "flow", "orchestr"}, "流程编排"},
		{[]string{"prompt"}, "提示词管理"},
		{[]string{"cli", "terminal", "shell"}, "命令行操作"},
		{[]string{"api", "sdk"}, "API 集成"},
	}
	for _, rule := range rules {
		for _, key := range rule.keys {
			if strings.Contains(corpus, key) {
				return rule.label
			}
		}
	}
	return ""
}

func toolURLKey(rawURL string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(strings.ToLower(rawURL))))
	return hex.EncodeToString(sum[:])
}

func cleanToolSummary(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "`\"'“”")
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "简介："))
	raw = strings.ReplaceAll(raw, "\n", " ")
	raw = strings.Join(strings.Fields(raw), " ")
	return truncateToolSummary(raw, 120)
}

func truncateToolSummary(raw string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(raw) <= limit {
		return raw
	}
	runes := []rune(raw)
	return strings.TrimSpace(string(runes[:limit])) + "..."
}

func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func optionalStringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
