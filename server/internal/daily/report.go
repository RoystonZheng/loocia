package daily

import "time"

// Wire models mirror docs/references/openapi.yaml DailyReport / DailyEntries.

type SectionItem struct {
	Title      string  `json:"title"`
	Summary    string  `json:"summary"`
	SourceURL  string  `json:"sourceUrl"`
	SourceName string  `json:"sourceName"`
	Permalink  *string `json:"permalink"`
}

type Section struct {
	Label string        `json:"label"`
	Items []SectionItem `json:"items"`
}

type Flash struct {
	Title       string     `json:"title"`
	SourceName  string     `json:"sourceName"`
	SourceURL   string     `json:"sourceUrl"`
	PublishedAt *time.Time `json:"publishedAt"`
	Permalink   *string    `json:"permalink"`
}

type Lead struct {
	Title         string `json:"title"`
	LeadParagraph string `json:"leadParagraph"`
}

type Report struct {
	Date        string    `json:"date"` // YYYY-MM-DD (UTC day)
	GeneratedAt time.Time `json:"generatedAt"`
	WindowStart time.Time `json:"windowStart"`
	WindowEnd   time.Time `json:"windowEnd"`
	Lead        *Lead     `json:"lead"`
	Sections    []Section `json:"sections"`
	Flashes     []Flash   `json:"flashes"`
}

// Entry is one row of the dailies archive index (openapi DailyEntries.items).
type Entry struct {
	Date          string    `json:"date"`
	GeneratedAt   time.Time `json:"generatedAt"`
	LeadTitle     *string   `json:"leadTitle"`
	LeadParagraph *string   `json:"leadParagraph"`
}

// categoryOrder is the fixed section order; categoryLabels maps slug → 中文 label
// (matches the openapi items↔daily correspondence table).
var categoryOrder = []string{"ai-models", "ai-products", "industry", "paper", "tip"}

var categoryLabels = map[string]string{
	"ai-models":   "模型发布/更新",
	"ai-products": "产品发布/更新",
	"industry":    "行业动态",
	"paper":       "论文研究",
	"tip":         "技巧与观点",
}
