package tools

import "time"

type Cursor struct {
	SortInt int
	SortAt  time.Time
	ID      string
}

type ListToolsParams struct {
	Status      ToolStatus
	Sort        SortMode
	Q           *string
	SourceType  *SourceType
	SourceTypes []SourceType
	PurposeTags []string
	Keyword     *string
	After       *Cursor
	Limit       int
	Offset      int
	Now         time.Time
}

type ToolListStats struct {
	Status                  ToolStatus
	Count                   int
	KeywordSourceCount      int
	TopicSourceCount        int
	ManualSourceCount       int
	LinkedEvaluationCount   int
	UnlinkedEvaluationCount int
	EvaluatorCount          int
	LatestUpdatedAt         *time.Time
	PurposeTags             []string
	Keywords                []string
}

type ToolListItem struct {
	Tool
	Sources    []ToolSourceSummary
	Reviews    []MemberReviewSummary
	Stars7D    *int
	Evaluation *EvaluationSummary
}

type ToolSourceSummary struct {
	SourceType SourceType
	Term       string
	ConfigID   *string
	LastSeenAt time.Time
	HitCount   int
}

type EvaluationSummary struct {
	ID                string
	Evaluator         string
	Operator          string
	CooperURL         *string
	StartedAt         time.Time
	CompletedAt       *time.Time
	Result            *EvaluationResult
	FinalSummary      *string
	NotIncludedReason *string
}

type MemberReview struct {
	ID        string
	ToolID    string
	Reviewer  string
	Operator  string
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type MemberReviewSummary struct {
	ID        string
	Reviewer  string
	Content   string
	CreatedAt time.Time
}
