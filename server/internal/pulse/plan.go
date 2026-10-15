package pulse

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ContentProfile describes the user's information need independently from the
// transport used to collect it. A source-plan file can therefore be reused for
// technology, policy, finance, research, or any other topic.
type ContentProfile struct {
	Name            string   `json:"name,omitempty"`
	Description     string   `json:"description,omitempty"`
	Language        string   `json:"language,omitempty"`
	IncludeKeywords []string `json:"include_keywords,omitempty"`
	ExcludeKeywords []string `json:"exclude_keywords,omitempty"`
	Categories      []string `json:"categories,omitempty"`
}

type SourcePlan struct {
	Profile ContentProfile `json:"profile,omitempty"`
	Sources []SourceConfig `json:"sources"`
}

func DefaultContentProfile() ContentProfile {
	return ContentProfile{
		Name:        "AI资讯",
		Description: "跟踪人工智能、大模型、AI 产品、研究和行业动态",
		Language:    "zh-CN",
		Categories:  []string{"ai-models", "ai-products", "industry", "paper", "tip"},
	}
}

// LoadPlan accepts the new object form and the historical bare source array.
// The compatibility path keeps existing deployments working while making the
// profile explicit for new users.
func LoadPlan(path string) (SourcePlan, error) {
	if strings.TrimSpace(path) == "" {
		return SourcePlan{Profile: DefaultContentProfile(), Sources: defaultSources}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return SourcePlan{}, fmt.Errorf("read sources file: %w", err)
	}
	var plan SourcePlan
	if err := json.Unmarshal(data, &plan); err == nil && plan.Sources != nil {
		if strings.TrimSpace(plan.Profile.Name) == "" {
			plan.Profile = DefaultContentProfile()
		}
		return plan, nil
	}
	var legacy []SourceConfig
	if err := json.Unmarshal(data, &legacy); err != nil {
		return SourcePlan{}, fmt.Errorf("parse sources file: %w", err)
	}
	return SourcePlan{Profile: DefaultContentProfile(), Sources: legacy}, nil
}
