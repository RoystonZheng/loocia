package tools

import "time"

type ToolStatus string

const (
	ToolDiscovered ToolStatus = "discovered"
	ToolEvaluating ToolStatus = "evaluating"
	ToolIncluded   ToolStatus = "included"
	ToolExcluded   ToolStatus = "excluded"
)

type DiscoveryMethod string

const (
	DiscoveryKeyword DiscoveryMethod = "keyword"
	DiscoveryTopic   DiscoveryMethod = "topic"
)

type TriggerMode string

const (
	TriggerManual TriggerMode = "manual"
	TriggerWeekly TriggerMode = "weekly"
)

type RunStatus string

const (
	RunRunning   RunStatus = "running"
	RunPaused    RunStatus = "paused"
	RunSucceeded RunStatus = "succeeded"
	RunPartial   RunStatus = "partial"
	RunFailed    RunStatus = "failed"
)

type SourceType string

const (
	SourceKeyword SourceType = "keyword"
	SourceTopic   SourceType = "topic"
	SourceManual  SourceType = "manual"
)

type SortMode string

const (
	SortLatest  SortMode = "latest"
	SortStars   SortMode = "stars"
	SortStars7D SortMode = "stars7d"
)

type GitHubTokenStrategy string

const (
	GitHubTokenStrategyRoundRobin GitHubTokenStrategy = "round_robin"
	GitHubTokenStrategyFixed      GitHubTokenStrategy = "fixed"
	GitHubTokenStrategyFailover   GitHubTokenStrategy = "failover"
)

type EvaluationResult string

const (
	EvaluationIncluded EvaluationResult = "included"
	EvaluationExcluded EvaluationResult = "excluded"
)

type DiscoveryConfig struct {
	ID                 string
	Name               string
	Method             DiscoveryMethod
	Terms              []string
	TriggerMode        TriggerMode
	Enabled            bool
	DeletedAt          *time.Time
	DeletedBy          *string
	LastSuccessAt      *time.Time
	LastRunAt          *time.Time
	LastRunStatus      *RunStatus
	LastResultCount    int
	LastPagesScanned   int
	LastPauseRequested bool
	LastFailureReason  *string
	CreatedBy          string
	UpdatedBy          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type DiscoveryRun struct {
	ID                string
	ConfigID          *string
	TriggerSource     TriggerMode
	Actor             string
	StartedAt         time.Time
	FinishedAt        *time.Time
	Status            RunStatus
	ResultCount       int
	NewCount          int
	UpdatedCount      int
	SkippedCount      int
	PagesScanned      int
	IncompleteResults bool
	Truncated         bool
	PauseRequested    bool
	NextQueryIndex    int
	NextPage          int
	ErrorClass        *string
	ErrorMessage      *string
	RateLimited       bool
	RateLimitResetAt  *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type RunResult struct {
	ID                string
	Status            RunStatus
	FinishedAt        *time.Time
	ResultCount       int
	NewCount          int
	UpdatedCount      int
	SkippedCount      int
	PagesScanned      int
	IncompleteResults bool
	Truncated         bool
	PauseRequested    bool
	NextQueryIndex    int
	NextPage          int
	ErrorClass        string
	ErrorMessage      string
	RateLimited       bool
	RateLimitResetAt  *time.Time
}

type ToolRuntimeSettings struct {
	ID                         string
	GitHubTokens               []string
	GitHubBaseURL              string
	GitHubTokenStrategy        GitHubTokenStrategy
	GitHubActiveTokenIndex     int
	IncludeDefaultGitHubTokens bool
	GitHubMaxPages             int
	GitHubPerPage              int
	GitHubRequestIntervalMS    int
	StarSnapshotLimit          int
	CreatedBy                  string
	UpdatedBy                  string
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

type GitHubRepo struct {
	NodeID                 string
	Owner                  string
	Repo                   string
	FullName               string
	URL                    string
	HomepageURL            string
	Name                   string
	Description            string
	Summary                string
	TemporarySummarySource string
	Stars                  int
	Forks                  int
	OpenIssues             int
	Topics                 []string
	LicenseSPDX            string
	DefaultBranch          string
	PushedAt               *time.Time
	Archived               bool
	Fork                   bool
	PurposeTags            []string
	PurposeTagsManuallySet bool
}

type DiscoverySource struct {
	ConfigID   *string
	SourceType SourceType
	Term       string
	Actor      string
}

type UpsertResult struct {
	Tool    Tool
	Created bool
}

type Tool struct {
	ID                     string
	GitHubNodeID           string
	GitHubOwner            string
	GitHubRepo             string
	GitHubFullName         string
	GitHubURL              string
	HomepageURL            *string
	Name                   string
	Description            *string
	TemporarySummary       *string
	TemporarySummarySource *string
	FinalSummary           *string
	Stars                  int
	Forks                  int
	OpenIssues             int
	Topics                 []string
	PurposeTags            []string
	PurposeTagsManuallySet bool
	LicenseSPDX            *string
	DefaultBranch          *string
	PushedAt               *time.Time
	Archived               bool
	Fork                   bool
	Status                 ToolStatus
	FirstDiscoveredAt      time.Time
	LastDiscoveredAt       time.Time
	IncludedAt             *time.Time
	ExcludedAt             *time.Time
	ExcludedStage          *string
	ExcludedReason         *string
	StatusVersion          int
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type StarSnapshot struct {
	ID         string
	ToolID     string
	Stars      int
	Forks      int
	OpenIssues int
	PushedAt   *time.Time
	CapturedOn time.Time
	SnapshotAt time.Time
}

type Evaluation struct {
	ID                string
	ToolID            string
	Evaluator         string
	Operator          string
	CooperURL         *string
	StartedAt         time.Time
	CompletedAt       *time.Time
	Result            *EvaluationResult
	FinalSummary      *string
	NotIncludedReason *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type FinishEvaluationRequest struct {
	ToolID            string
	EvaluationID      string
	Result            EvaluationResult
	Operator          string
	FinalSummary      string
	NotIncludedReason string
}
