package tools

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	DefaultGitHubMaxPages         = 5
	DefaultGitHubPerPage          = 100
	DefaultGitHubRequestInterval  = 2 * time.Second
	DefaultStarSnapshotLimit      = 200
	DefaultPurposeClassifyTimeout = 2 * time.Second
	finishRunTimeout              = 10 * time.Second
)

type GitHubClient interface {
	SearchRepositories(ctx context.Context, req GitHubSearchRequest) (GitHubSearchResult, error)
	GetRepository(ctx context.Context, owner, repo string) (GitHubRepo, error)
}

type GitHubQuery struct {
	Term       string
	SourceType SourceType
	Query      string
	Sort       string
	Order      string
}

type Discoverer struct {
	Store                  *Store
	GitHub                 GitHubClient
	MaxPages               int
	PerPage                int
	RequestInterval        time.Duration
	StarSnapshotLimit      int
	PurposeClassifier      PurposeClassifier
	PurposeClassifyTimeout time.Duration
	Sleep                  func(context.Context, time.Duration) error
	Now                    func() time.Time
}

type WeeklySummary struct {
	Attempted int
	Succeeded int
	Partial   int
	Failed    int
}

type StarSnapshotSummary struct {
	Attempted   int
	Saved       int
	Skipped     int
	Failed      int
	RateLimited bool
}

type PurposeReclassifySummary struct {
	Attempted int
	Updated   int
	Skipped   int
	Failed    int
}

type ManualAddResult struct {
	Tool      Tool
	Created   bool
	Duplicate bool
}

func NewDiscoverer(store *Store, github GitHubClient) *Discoverer {
	return &Discoverer{
		Store:                  store,
		GitHub:                 github,
		MaxPages:               DefaultGitHubMaxPages,
		PerPage:                DefaultGitHubPerPage,
		RequestInterval:        DefaultGitHubRequestInterval,
		StarSnapshotLimit:      DefaultStarSnapshotLimit,
		PurposeClassifyTimeout: DefaultPurposeClassifyTimeout,
		Now:                    time.Now,
	}
}

func (d *Discoverer) ApplyRuntimeSettings(ctx context.Context) error {
	if d == nil || d.Store == nil {
		return nil
	}
	settings, found, err := d.Store.FindRuntimeSettings(ctx)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if client, ok := d.GitHub.(*HTTPGitHubClient); ok {
		client.SetTokens(settings.GitHubTokens)
		client.SetTokenSelection(settings.GitHubTokenStrategy, settings.GitHubActiveTokenIndex, settings.IncludeDefaultGitHubTokens)
		if settings.GitHubBaseURL == "" {
			client.BaseURL = GitHubDefaultBaseURL
		} else {
			client.BaseURL = settings.GitHubBaseURL
		}
	}
	d.MaxPages = settings.GitHubMaxPages
	d.PerPage = settings.GitHubPerPage
	d.RequestInterval = time.Duration(settings.GitHubRequestIntervalMS) * time.Millisecond
	d.StarSnapshotLimit = settings.StarSnapshotLimit
	return nil
}

func BuildGitHubQueries(cfg DiscoveryConfig) []GitHubQuery {
	terms := normalizedTerms(cfg.Terms)
	out := make([]GitHubQuery, 0, len(terms)*2)
	for _, term := range terms {
		base, source := githubBaseQuery(cfg.Method, term)
		if base == "" {
			continue
		}
		if cfg.LastSuccessAt == nil {
			out = append(out, GitHubQuery{
				Term:       term,
				SourceType: source,
				Query:      base,
				Sort:       "stars",
				Order:      "desc",
			})
			continue
		}
		since := cfg.LastSuccessAt.Add(-24 * time.Hour).UTC().Format(time.RFC3339)
		out = append(out,
			GitHubQuery{Term: term, SourceType: source, Query: base + " created:>" + since},
			GitHubQuery{Term: term, SourceType: source, Query: base + " pushed:>" + since},
		)
	}
	return out
}

func githubBaseQuery(method DiscoveryMethod, term string) (string, SourceType) {
	switch method {
	case DiscoveryKeyword:
		return fmt.Sprintf("%q in:name,description,topics,readme fork:false archived:false", term), SourceKeyword
	case DiscoveryTopic:
		return "topic:" + term + " fork:false archived:false", SourceTopic
	default:
		return "", ""
	}
}

func normalizedTerms(terms []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		key := strings.ToLower(term)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, term)
	}
	return out
}

func (d *Discoverer) RunConfig(ctx context.Context, configID string, trigger TriggerMode, actor string) (RunResult, error) {
	if d == nil || d.Store == nil || d.GitHub == nil {
		return RunResult{}, &DiscoveryError{Class: ErrorGitHub, Message: "discoverer dependencies are not configured"}
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "system"
	}
	if trigger == "" {
		trigger = TriggerManual
	}
	started, err := d.StartConfigRun(ctx, configID, trigger, actor)
	if err != nil {
		return RunResult{}, err
	}
	return d.RunExistingConfig(ctx, configID, started.ID)
}

func (d *Discoverer) StartConfigRun(ctx context.Context, configID string, trigger TriggerMode, actor string) (RunResult, error) {
	if d == nil || d.Store == nil || d.GitHub == nil {
		return RunResult{}, &DiscoveryError{Class: ErrorGitHub, Message: "discoverer dependencies are not configured"}
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "system"
	}
	if trigger == "" {
		trigger = TriggerManual
	}
	cfg, err := d.Store.GetConfig(ctx, configID)
	if err != nil {
		return RunResult{}, err
	}

	runID := newID("run")
	configRef := cfg.ID
	if err := d.Store.CreateRun(ctx, DiscoveryRun{
		ID:            runID,
		ConfigID:      &configRef,
		TriggerSource: trigger,
		Actor:         actor,
		Status:        RunRunning,
		NextPage:      1,
	}); err != nil {
		return RunResult{}, err
	}

	return RunResult{ID: runID, Status: RunRunning, NextPage: 1}, nil
}

func (d *Discoverer) RequestConfigPause(ctx context.Context, configID, actor string) (RunResult, error) {
	if d == nil || d.Store == nil {
		return RunResult{}, &DiscoveryError{Class: ErrorGitHub, Message: "discoverer dependencies are not configured"}
	}
	run, err := d.Store.RequestRunPause(ctx, configID, strings.TrimSpace(actor))
	if err != nil {
		return RunResult{}, err
	}
	return runResultFromRun(run), nil
}

func (d *Discoverer) ResumeConfigRun(ctx context.Context, configID, actor string) (RunResult, error) {
	if d == nil || d.Store == nil || d.GitHub == nil {
		return RunResult{}, &DiscoveryError{Class: ErrorGitHub, Message: "discoverer dependencies are not configured"}
	}
	run, err := d.Store.ResumeRun(ctx, configID, strings.TrimSpace(actor))
	if err != nil {
		return RunResult{}, err
	}
	return runResultFromRun(run), nil
}

func (d *Discoverer) RunExistingConfig(ctx context.Context, configID, runID string) (RunResult, error) {
	if d == nil || d.Store == nil || d.GitHub == nil {
		return RunResult{}, &DiscoveryError{Class: ErrorGitHub, Message: "discoverer dependencies are not configured"}
	}
	if err := d.ApplyRuntimeSettings(ctx); err != nil {
		return RunResult{}, err
	}
	cfg, err := d.Store.GetConfig(ctx, configID)
	if err != nil {
		return RunResult{}, err
	}
	run, err := d.Store.GetRun(ctx, runID)
	if err != nil {
		return RunResult{}, err
	}
	result := runResultFromRun(run)
	if run.ConfigID == nil || strings.TrimSpace(*run.ConfigID) != cfg.ID {
		return result, statusConflict("discovery run does not belong to config")
	}
	if run.FinishedAt != nil {
		return result, statusConflict("discovery run has already finished")
	}
	if run.Status == RunPaused {
		return result, nil
	}
	if run.Status != RunRunning {
		return result, statusConflict("discovery run is not running")
	}
	configRef := cfg.ID
	actor := strings.TrimSpace(run.Actor)
	if actor == "" {
		actor = "system"
	}

	queries := BuildGitHubQueries(cfg)
	if len(queries) == 0 {
		result.Status = RunFailed
		result.ErrorClass = ErrorBadQuery
		result.ErrorMessage = "no valid discovery terms"
		return result, d.finishRun(ctx, result)
	}

	maxPages := d.MaxPages
	if maxPages <= 0 {
		maxPages = DefaultGitHubMaxPages
	}
	perPage := d.PerPage
	if perPage <= 0 {
		perPage = DefaultGitHubPerPage
	}

	errorSeen := false
	requestSeen := result.PagesScanned > 0
	stopDiscovery := false

	startQueryIndex := nonNegative(run.NextQueryIndex)
	if startQueryIndex > len(queries) {
		startQueryIndex = len(queries)
	}
	startPage := positiveOrDefault(run.NextPage, 1)
	if startQueryIndex >= len(queries) {
		result.Status = RunSucceeded
		result.NextQueryIndex = len(queries)
		result.NextPage = 1
		return result, d.finishRun(ctx, result)
	}

	for queryIndex := startQueryIndex; queryIndex < len(queries); queryIndex++ {
		q := queries[queryIndex]
		firstPage := 1
		if queryIndex == startQueryIndex {
			firstPage = startPage
		}
		for page := firstPage; page <= maxPages; page++ {
			result.NextQueryIndex = queryIndex
			result.NextPage = page
			paused, err := d.pauseIfRequested(ctx, &result)
			if err != nil || paused {
				return result, err
			}
			if err := d.waitBetweenGitHubRequests(ctx, requestSeen); err != nil {
				result.Status = RunFailed
				result.ErrorClass = ErrorNetwork
				result.ErrorMessage = err.Error()
				return result, d.finishRun(ctx, result)
			}
			requestSeen = true
			search, err := d.GitHub.SearchRepositories(ctx, GitHubSearchRequest{
				Query:   q.Query,
				Page:    page,
				PerPage: perPage,
				Sort:    q.Sort,
				Order:   q.Order,
			})
			if err != nil {
				errorSeen = true
				applyDiscoveryError(&result, err)
				if result.RateLimited {
					stopDiscovery = true
				}
				break
			}

			result.PagesScanned++
			if search.IncompleteResults {
				result.IncompleteResults = true
			}
			for _, repo := range search.Repos {
				if repo.Archived || repo.Fork {
					result.SkippedCount++
					continue
				}
				repo.PurposeTags = d.classifyPurposeTags(ctx, repo)
				upsert, err := d.Store.UpsertFromGitHub(ctx, repo, DiscoverySource{
					ConfigID:   &configRef,
					SourceType: q.SourceType,
					Term:       q.Term,
					Actor:      actor,
				})
				if err != nil {
					return d.failRun(ctx, result, err)
				}
				result.ResultCount++
				if upsert.Created {
					result.NewCount++
				} else {
					result.UpdatedCount++
				}
			}
			result.NextQueryIndex = queryIndex
			result.NextPage = page + 1
			if search.TotalCount > page*perPage && page == maxPages {
				result.Truncated = true
			}
			if len(search.Repos) < perPage || page == maxPages {
				result.NextQueryIndex = queryIndex + 1
				result.NextPage = 1
			}
			if err := d.persistRunProgress(ctx, result); err != nil {
				return d.failRun(ctx, result, err)
			}
			paused, err = d.pauseIfRequested(ctx, &result)
			if err != nil || paused {
				return result, err
			}
			if len(search.Repos) < perPage {
				break
			}
		}
		if stopDiscovery {
			break
		}
	}

	result.Status = runStatusForDiscovery(result, errorSeen)
	result.NextQueryIndex = len(queries)
	result.NextPage = 1
	if result.ErrorClass == "" {
		switch {
		case result.Truncated:
			result.ErrorClass = ErrorTruncated
			result.ErrorMessage = "GitHub search reached the configured page limit"
		case result.IncompleteResults:
			result.ErrorClass = "incomplete_results"
			result.ErrorMessage = "GitHub search returned incomplete results"
		}
	}
	return result, d.finishRun(ctx, result)
}

func (d *Discoverer) pauseIfRequested(ctx context.Context, result *RunResult) (bool, error) {
	pauseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finishRunTimeout)
	defer cancel()
	pauseRequested, err := d.Store.IsRunPauseRequested(pauseCtx, result.ID)
	if err != nil {
		return false, err
	}
	if !pauseRequested {
		return false, nil
	}
	result.Status = RunPaused
	result.PauseRequested = false
	if err := d.pauseRun(ctx, *result); err != nil {
		return false, err
	}
	return true, nil
}

func (d *Discoverer) pauseRun(ctx context.Context, result RunResult) error {
	pauseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finishRunTimeout)
	defer cancel()
	return d.Store.PauseRun(pauseCtx, result)
}

func (d *Discoverer) persistRunProgress(ctx context.Context, result RunResult) error {
	progressCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finishRunTimeout)
	defer cancel()
	return d.Store.UpdateRunProgress(progressCtx, result)
}

func (d *Discoverer) failRun(ctx context.Context, result RunResult, err error) (RunResult, error) {
	result.Status = runStatusForDiscovery(result, true)
	applyDiscoveryError(&result, err)
	if finishErr := d.finishRun(ctx, result); finishErr != nil {
		return result, finishErr
	}
	return result, err
}

func (d *Discoverer) finishRun(ctx context.Context, result RunResult) error {
	if result.FinishedAt == nil {
		now := d.now().UTC()
		result.FinishedAt = &now
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finishRunTimeout)
	defer cancel()
	return d.Store.FinishRun(finishCtx, result)
}

func (d *Discoverer) waitBetweenGitHubRequests(ctx context.Context, requestSeen bool) error {
	if !requestSeen || d == nil || d.RequestInterval <= 0 {
		return nil
	}
	sleep := d.Sleep
	if sleep == nil {
		sleep = sleepContext
	}
	return sleep(ctx, d.RequestInterval)
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (d *Discoverer) now() time.Time {
	if d != nil && d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func runStatusForDiscovery(result RunResult, errorSeen bool) RunStatus {
	if errorSeen || result.Truncated || result.IncompleteResults {
		if result.ResultCount > 0 || result.PagesScanned > 0 {
			return RunPartial
		}
		return RunFailed
	}
	return RunSucceeded
}

func applyDiscoveryError(result *RunResult, err error) {
	var derr *DiscoveryError
	if !AsDiscoveryError(err, &derr) {
		derr = &DiscoveryError{Class: ErrorGitHub, Message: err.Error()}
	}
	if result.ErrorClass == "" {
		result.ErrorClass = derr.Class
		result.ErrorMessage = derr.Message
	}
	if derr.RateLimited {
		result.RateLimited = true
		if result.RateLimitResetAt == nil {
			result.RateLimitResetAt = derr.RateLimitResetAt
		}
	}
}

func runResultFromRun(run DiscoveryRun) RunResult {
	result := RunResult{
		ID:                run.ID,
		Status:            run.Status,
		FinishedAt:        run.FinishedAt,
		ResultCount:       run.ResultCount,
		NewCount:          run.NewCount,
		UpdatedCount:      run.UpdatedCount,
		SkippedCount:      run.SkippedCount,
		PagesScanned:      run.PagesScanned,
		IncompleteResults: run.IncompleteResults,
		Truncated:         run.Truncated,
		PauseRequested:    run.PauseRequested,
		NextQueryIndex:    nonNegative(run.NextQueryIndex),
		NextPage:          positiveOrDefault(run.NextPage, 1),
		RateLimited:       run.RateLimited,
		RateLimitResetAt:  run.RateLimitResetAt,
	}
	if run.ErrorClass != nil {
		result.ErrorClass = *run.ErrorClass
	}
	if run.ErrorMessage != nil {
		result.ErrorMessage = *run.ErrorMessage
	}
	return result
}

func (d *Discoverer) RunWeekly(ctx context.Context, actor string) (WeeklySummary, error) {
	configs, err := d.Store.ListRunnableConfigs(ctx, TriggerWeekly)
	if err != nil {
		return WeeklySummary{}, err
	}
	var summary WeeklySummary
	for _, cfg := range configs {
		summary.Attempted++
		result, err := d.RunConfig(ctx, cfg.ID, TriggerWeekly, actor)
		if err != nil {
			summary.Failed++
			continue
		}
		switch result.Status {
		case RunSucceeded:
			summary.Succeeded++
		case RunPartial:
			summary.Partial++
		case RunFailed:
			summary.Failed++
		case RunPaused:
			summary.Partial++
		}
	}
	return summary, nil
}

func (d *Discoverer) SnapshotActiveTools(ctx context.Context, limit int) (StarSnapshotSummary, error) {
	if d == nil || d.Store == nil || d.GitHub == nil {
		return StarSnapshotSummary{}, &DiscoveryError{Class: ErrorGitHub, Message: "discoverer dependencies are not configured"}
	}
	if err := d.ApplyRuntimeSettings(ctx); err != nil {
		return StarSnapshotSummary{}, err
	}
	if limit <= 0 {
		limit = d.StarSnapshotLimit
	}
	if limit <= 0 {
		limit = DefaultStarSnapshotLimit
	}

	now := d.now().UTC()
	activeTools, err := d.Store.ListToolsMissingStarSnapshot(ctx, now, limit)
	if err != nil {
		return StarSnapshotSummary{}, err
	}

	var summary StarSnapshotSummary
	requestSeen := false
	for _, tool := range activeTools {
		if err := d.waitBetweenGitHubRequests(ctx, requestSeen); err != nil {
			summary.Failed++
			return summary, err
		}
		requestSeen = true
		summary.Attempted++

		repo, err := d.GitHub.GetRepository(ctx, tool.GitHubOwner, tool.GitHubRepo)
		if err != nil {
			summary.Failed++
			var derr *DiscoveryError
			if AsDiscoveryError(err, &derr) && derr.RateLimited {
				summary.RateLimited = true
				break
			}
			continue
		}
		if repo.Archived || repo.Fork {
			summary.Skipped++
			continue
		}
		if repo.NodeID != "" && tool.GitHubNodeID != "" && repo.NodeID != tool.GitHubNodeID {
			summary.Failed++
			continue
		}
		if err := d.Store.SaveStarSnapshot(ctx, StarSnapshot{
			ID:         newID("snap"),
			ToolID:     tool.ID,
			Stars:      repo.Stars,
			Forks:      repo.Forks,
			OpenIssues: repo.OpenIssues,
			PushedAt:   repo.PushedAt,
			CapturedOn: now,
			SnapshotAt: now,
		}); err != nil {
			summary.Failed++
			return summary, err
		}
		summary.Saved++
	}
	return summary, nil
}

func (d *Discoverer) ReclassifyPurposeTags(ctx context.Context, limit int) (PurposeReclassifySummary, error) {
	if d == nil || d.Store == nil {
		return PurposeReclassifySummary{}, &DiscoveryError{Class: ErrorGitHub, Message: "tool store is not configured"}
	}
	tools, err := d.Store.ListToolsForPurposeClassification(ctx, limit)
	if err != nil {
		return PurposeReclassifySummary{}, err
	}
	var summary PurposeReclassifySummary
	for _, tool := range tools {
		summary.Attempted++
		if tool.PurposeTagsManuallySet {
			summary.Skipped++
			continue
		}
		tags := d.classifyPurposeTags(ctx, RepoFromToolForPurpose(tool))
		if err := d.Store.UpdateToolPurposeTags(ctx, tool.ID, tags, false); err != nil {
			summary.Failed++
			return summary, err
		}
		summary.Updated++
	}
	return summary, nil
}

func (d *Discoverer) PreviewManualAdd(ctx context.Context, repoURL string) (GitHubRepo, error) {
	if d == nil || d.GitHub == nil {
		return GitHubRepo{}, &DiscoveryError{Class: ErrorGitHub, Message: "github client is not configured"}
	}
	if err := d.ApplyRuntimeSettings(ctx); err != nil {
		return GitHubRepo{}, err
	}
	owner, repo, err := ParseGitHubRepoURL(repoURL)
	if err != nil {
		return GitHubRepo{}, err
	}
	ghRepo, err := d.GitHub.GetRepository(ctx, owner, repo)
	if err != nil {
		return GitHubRepo{}, err
	}
	if ghRepo.Archived {
		return GitHubRepo{}, &DiscoveryError{Class: ErrorRepositoryRejected, Message: "archived repositories are not accepted"}
	}
	if ghRepo.Fork {
		return GitHubRepo{}, &DiscoveryError{Class: ErrorRepositoryRejected, Message: "fork repositories are not accepted"}
	}
	ghRepo.PurposeTags = d.classifyPurposeTags(ctx, ghRepo)
	return ghRepo, nil
}

func (d *Discoverer) classifyPurposeTags(ctx context.Context, repo GitHubRepo) []string {
	if len(NormalizePurposeTags(repo.PurposeTags)) > 0 || repo.PurposeTagsManuallySet {
		return NormalizePurposeTags(repo.PurposeTags)
	}
	if d != nil && d.PurposeClassifier != nil {
		timeout := d.PurposeClassifyTimeout
		if timeout <= 0 {
			timeout = DefaultPurposeClassifyTimeout
		}
		classifyCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if tags, err := d.PurposeClassifier.ClassifyPurposeTags(classifyCtx, repo, d.purposeTagOptions(ctx)); err == nil {
			return NormalizePurposeTags(tags)
		}
	}
	return InferPurposeTags(repo)
}

func (d *Discoverer) purposeTagOptions(ctx context.Context) []string {
	options := append([]string{}, DefaultPurposeTags...)
	if d == nil || d.Store == nil {
		return options
	}
	tags, err := d.Store.ListPurposeTags(ctx, ListToolsParams{})
	if err != nil {
		return options
	}
	options = append(options, tags...)
	return NormalizePurposeTags(options)
}

func (d *Discoverer) ManualAdd(ctx context.Context, repoURL, actor string, purposeTags ...[]string) (ManualAddResult, error) {
	if d == nil || d.Store == nil {
		return ManualAddResult{}, &DiscoveryError{Class: ErrorGitHub, Message: "tool store is not configured"}
	}
	ghRepo, err := d.PreviewManualAdd(ctx, repoURL)
	if err != nil {
		return ManualAddResult{}, err
	}
	upsert, err := d.Store.ManualAdd(ctx, ghRepo, actor, purposeTags...)
	if err != nil {
		return ManualAddResult{}, err
	}
	return ManualAddResult{
		Tool:      upsert.Tool,
		Created:   upsert.Created,
		Duplicate: !upsert.Created,
	}, nil
}

func (d *Discoverer) ImportTeamTool(ctx context.Context, repoURL, operator, finalSummary, cooperURL string, purposeTags ...[]string) (ManualAddResult, error) {
	if d == nil || d.Store == nil {
		return ManualAddResult{}, &DiscoveryError{Class: ErrorGitHub, Message: "tool store is not configured"}
	}
	ghRepo, err := d.PreviewManualAdd(ctx, repoURL)
	if err != nil {
		return ManualAddResult{}, err
	}
	upsert, err := d.Store.ImportTeamTool(ctx, ghRepo, operator, finalSummary, cooperURL, purposeTags...)
	if err != nil {
		return ManualAddResult{}, err
	}
	return ManualAddResult{
		Tool:      upsert.Tool,
		Created:   upsert.Created,
		Duplicate: !upsert.Created,
	}, nil
}
