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
}

type ToolListItem struct {
	Tool
	Sources    []ToolSourceSummary
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
