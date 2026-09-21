package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const MaxPurposeTags = 8

var DefaultPurposeTags = []string{
	"代码开发",
	"浏览器操作",
	"深度研究",
	"知识库检索",
	"MCP 集成",
	"Agent 编排",
	"数据分析",
	"工作流自动化",
	"测试验证",
	"文档写作",
	"设计创作",
	"命令行效率",
	"API/SDK 集成",
	"提示词管理",
}

type purposeRule struct {
	label    string
	keywords []string
}

type PurposeClassifier interface {
	ClassifyPurposeTags(ctx context.Context, repo GitHubRepo, options []string) ([]string, error)
}

type purposeLLM interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

type LLMPurposeClassifier struct {
	LLM purposeLLM
}

func NewLLMPurposeClassifier(llm purposeLLM) LLMPurposeClassifier {
	return LLMPurposeClassifier{LLM: llm}
}

var purposeRules = []purposeRule{
	{label: "浏览器操作", keywords: []string{"browser", "computer-use", "computer use", "web automation", "playwright", "selenium", "puppeteer", "网页", "浏览器", "自动操作"}},
	{label: "代码开发", keywords: []string{"coding", "code", "developer", "review", "programming", "ide", "devtools", "代码", "编程", "研发", "代码审查"}},
	{label: "深度研究", keywords: []string{"research", "deepsearch", "deep research", "search", "literature", "paper", "citation", "资料", "研究", "检索", "论文"}},
	{label: "知识库检索", keywords: []string{"rag", "retrieval", "embedding", "vector", "knowledge", "semantic search", "知识库", "向量", "语义检索"}},
	{label: "MCP 集成", keywords: []string{"mcp", "model-context-protocol", "model context protocol", "context protocol"}},
	{label: "Agent 编排", keywords: []string{"agent", "agents", "autonomous", "multi-agent", "multi agent", "swarm", "agentic", "智能体", "多智能体"}},
	{label: "数据分析", keywords: []string{"data", "analytics", "analysis", "visualization", "notebook", "warehouse", "sql", "数据", "分析", "可视化"}},
	{label: "工作流自动化", keywords: []string{"workflow", "flow", "orchestr", "automation", "pipeline", "zapier", "n8n", "流程", "编排", "自动化"}},
	{label: "测试验证", keywords: []string{"test", "testing", "qa", "eval", "evaluation", "benchmark", "验证", "测试", "评测"}},
	{label: "文档写作", keywords: []string{"doc", "docs", "document", "writing", "markdown", "note", "文档", "写作", "笔记"}},
	{label: "设计创作", keywords: []string{"design", "image", "video", "creative", "ui", "figma", "drawing", "设计", "图片", "视频", "创作"}},
	{label: "命令行效率", keywords: []string{"cli", "terminal", "shell", "command line", "命令行", "终端", "脚本"}},
	{label: "API/SDK 集成", keywords: []string{"api", "sdk", "integration", "client", "server", "proxy", "接口", "集成"}},
	{label: "提示词管理", keywords: []string{"prompt", "prompts", "prompting", "提示词"}},
}

// InferPurposeTags classifies a repository by usage scenario. It deliberately
// returns no tag when signals are too weak, so unclear tools are not forced into
// a misleading category.
func InferPurposeTags(repo GitHubRepo) []string {
	corpus := strings.ToLower(strings.Join(toolPurposeCorpus(repo), " "))
	if strings.TrimSpace(corpus) == "" {
		return nil
	}

	type scored struct {
		label string
		score int
		order int
	}
	var scores []scored
	for i, rule := range purposeRules {
		score := 0
		for _, keyword := range rule.keywords {
			if strings.Contains(corpus, strings.ToLower(keyword)) {
				score++
			}
		}
		if score > 0 {
			scores = append(scores, scored{label: rule.label, score: score, order: i})
		}
	}
	if len(scores) == 0 {
		return nil
	}
	sort.SliceStable(scores, func(i, j int) bool {
		if scores[i].score == scores[j].score {
			return scores[i].order < scores[j].order
		}
		return scores[i].score > scores[j].score
	})

	limit := 3
	if len(scores) < limit {
		limit = len(scores)
	}
	out := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, scores[i].label)
	}
	return out
}

func (c LLMPurposeClassifier) ClassifyPurposeTags(ctx context.Context, repo GitHubRepo, options []string) ([]string, error) {
	if c.LLM == nil {
		return nil, fmt.Errorf("llm is nil")
	}
	options = NormalizePurposeTags(options)
	if len(options) == 0 {
		options = DefaultPurposeTags
	}
	system := "你是 AI Cool 的工具用途分类器。请按使用场景给 GitHub 工具分类，最多 3 个标签。优先从候选标签里选择；如果候选都不符合，可以返回新标签；如果信息不足，不要强行分类。只输出 JSON，不要解释。JSON 格式：{\"purpose_tags\":[\"标签\"]}。"
	user := fmt.Sprintf("候选标签：%s\n仓库：%s\n名称：%s\n描述：%s\nTopics：%s\n补充说明：%s",
		strings.Join(options, "、"),
		repo.FullName,
		repo.Name,
		strings.TrimSpace(repo.Description),
		strings.Join(repo.Topics, ", "),
		strings.TrimSpace(repo.Summary),
	)
	raw, err := c.LLM.Complete(ctx, system, user)
	if err != nil {
		return nil, err
	}
	var out struct {
		PurposeTags      []string `json:"purpose_tags"`
		PurposeTagsCamel []string `json:"purposeTags"`
	}
	if err := json.Unmarshal([]byte(extractPurposeJSON(raw)), &out); err != nil {
		return nil, err
	}
	tags := out.PurposeTags
	if len(tags) == 0 {
		tags = out.PurposeTagsCamel
	}
	return NormalizePurposeTags(tags), nil
}

func extractPurposeJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end >= start {
		return raw[start : end+1]
	}
	return raw
}

func NormalizePurposeTags(tags []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = normalizePurposeTag(tag)
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tag)
		if len(out) >= MaxPurposeTags {
			break
		}
	}
	return out
}

func normalizePurposeTag(tag string) string {
	tag = strings.TrimSpace(tag)
	tag = strings.Join(strings.Fields(tag), " ")
	tag = strings.Trim(tag, "，,;；、")
	if utf8.RuneCountInString(tag) > 24 {
		runes := []rune(tag)
		tag = string(runes[:24])
	}
	return strings.TrimSpace(tag)
}

func toolPurposeCorpus(repo GitHubRepo) []string {
	parts := []string{
		repo.Name,
		repo.FullName,
		repo.Description,
		repo.Summary,
		repo.TemporarySummarySource,
	}
	parts = append(parts, repo.Topics...)
	return parts
}

func RepoFromToolForPurpose(tool Tool) GitHubRepo {
	description := ""
	if tool.Description != nil {
		description = *tool.Description
	}
	summary := ""
	if tool.FinalSummary != nil {
		summary = *tool.FinalSummary
	} else if tool.TemporarySummary != nil {
		summary = *tool.TemporarySummary
	}
	summarySource := ""
	if tool.TemporarySummarySource != nil {
		summarySource = *tool.TemporarySummarySource
	}
	return GitHubRepo{
		NodeID:                 tool.GitHubNodeID,
		Owner:                  tool.GitHubOwner,
		Repo:                   tool.GitHubRepo,
		FullName:               tool.GitHubFullName,
		URL:                    tool.GitHubURL,
		Name:                   tool.Name,
		Description:            description,
		Summary:                summary,
		TemporarySummarySource: summarySource,
		Topics:                 tool.Topics,
	}
}
