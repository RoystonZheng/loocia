package tools

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeGitHubClient struct {
	searches map[string]GitHubSearchResult
	errors   map[string]error
	repos    map[string]GitHubRepo
}

func (f fakeGitHubClient) SearchRepositories(ctx context.Context, req GitHubSearchRequest) (GitHubSearchResult, error) {
	if err := f.errors[req.Query]; err != nil {
		return GitHubSearchResult{}, err
	}
	return f.searches[req.Query], nil
}

func (f fakeGitHubClient) GetRepository(ctx context.Context, owner, repo string) (GitHubRepo, error) {
	key := owner + "/" + repo
	if err := f.errors[key]; err != nil {
		return GitHubRepo{}, err
	}
	got, ok := f.repos[key]
	if !ok {
		return GitHubRepo{}, &DiscoveryError{Class: ErrorGitHub, Message: "not found"}
	}
	return got, nil
}

type recordingGitHubClient struct {
	searches map[string]GitHubSearchResult
	calls    []GitHubSearchRequest
}

func (f *recordingGitHubClient) SearchRepositories(ctx context.Context, req GitHubSearchRequest) (GitHubSearchResult, error) {
	f.calls = append(f.calls, req)
	return f.searches[req.Query], nil
}

func (f *recordingGitHubClient) GetRepository(ctx context.Context, owner, repo string) (GitHubRepo, error) {
	return GitHubRepo{}, &DiscoveryError{Class: ErrorGitHub, Message: "not found"}
}

type cancelingGitHubClient struct {
	cancel context.CancelFunc
}

func (f cancelingGitHubClient) SearchRepositories(ctx context.Context, req GitHubSearchRequest) (GitHubSearchResult, error) {
	f.cancel()
	<-ctx.Done()
	return GitHubSearchResult{}, ctx.Err()
}

func (f cancelingGitHubClient) GetRepository(ctx context.Context, owner, repo string) (GitHubRepo, error) {
	return GitHubRepo{}, &DiscoveryError{Class: ErrorGitHub, Message: "not found"}
}

type cancelBeforeUpsertGitHubClient struct {
	cancel context.CancelFunc
}

func (f cancelBeforeUpsertGitHubClient) SearchRepositories(ctx context.Context, req GitHubSearchRequest) (GitHubSearchResult, error) {
	f.cancel()
	return GitHubSearchResult{
		TotalCount: 1,
		Repos:      []GitHubRepo{sampleRepo("node-cancel-upsert", "openai", "browser-agent")},
	}, nil
}

func (f cancelBeforeUpsertGitHubClient) GetRepository(ctx context.Context, owner, repo string) (GitHubRepo, error) {
	return GitHubRepo{}, &DiscoveryError{Class: ErrorGitHub, Message: "not found"}
}

func TestDiscovererRunConfigPersistsSearchResultsAndRunSummary(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cfg := DiscoveryConfig{
		ID:          "cfg-agents",
		Name:        "Agent tools",
		Method:      DiscoveryTopic,
		Terms:       []string{"ai-agents"},
		TriggerMode: TriggerManual,
		Enabled:     true,
		CreatedBy:   "alice",
		UpdatedBy:   "alice",
	}
	if err := s.UpsertConfig(ctx, cfg); err != nil {
		t.Fatalf("UpsertConfig: %v", err)
	}

	gh := fakeGitHubClient{searches: map[string]GitHubSearchResult{
		"topic:ai-agents fork:false archived:false": {
			TotalCount: 1,
			Repos: []GitHubRepo{
				sampleRepo("node-1", "openai", "agents"),
				func() GitHubRepo {
					r := sampleRepo("node-archived", "old", "archived")
					r.Archived = true
					return r
				}(),
			},
		},
	}}
	d := NewDiscoverer(s, gh)
	d.MaxPages = 2
	result, err := d.RunConfig(ctx, cfg.ID, TriggerManual, "alice")
	if err != nil {
		t.Fatalf("RunConfig: %v", err)
	}
	if result.Status != RunSucceeded || result.ResultCount != 1 || result.NewCount != 1 || result.SkippedCount != 1 || result.PagesScanned != 1 {
		t.Fatalf("run result mismatch: %+v", result)
	}

	items, err := s.ListTools(ctx, ListToolsParams{Status: ToolDiscovered, Limit: 10})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(items) != 1 || items[0].GitHubNodeID != "node-1" {
		t.Fatalf("tool mismatch: %+v", items)
	}
	if len(items[0].Sources) != 1 || items[0].Sources[0].SourceType != SourceTopic || items[0].Sources[0].Term != "ai-agents" {
		t.Fatalf("source mismatch: %+v", items[0].Sources)
	}

	configs, err := s.ListConfigs(ctx)
	if err != nil {
		t.Fatalf("ListConfigs: %v", err)
	}
	if configs[0].LastRunStatus == nil || *configs[0].LastRunStatus != RunSucceeded || configs[0].LastSuccessAt == nil {
		t.Fatalf("config run info mismatch: %+v", configs[0])
	}
}

func TestDiscovererRunConfigClosesRunWhenRequestContextIsCanceled(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cfg := DiscoveryConfig{
		ID:          "cfg-cancelled",
		Name:        "Cancelled run",
		Method:      DiscoveryKeyword,
		Terms:       []string{"browser agent"},
		TriggerMode: TriggerManual,
		Enabled:     true,
		CreatedBy:   "alice",
		UpdatedBy:   "alice",
	}
	if err := s.UpsertConfig(ctx, cfg); err != nil {
		t.Fatalf("UpsertConfig: %v", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	d := NewDiscoverer(s, cancelingGitHubClient{cancel: cancel})
	d.RequestInterval = 0
	result, err := d.RunConfig(runCtx, cfg.ID, TriggerManual, "alice")
	if err != nil {
		t.Fatalf("RunConfig should finish the run even after request cancellation: %v", err)
	}
	if result.Status != RunFailed || result.ErrorMessage != context.Canceled.Error() {
		t.Fatalf("cancelled run result mismatch: %+v", result)
	}

	configs, err := s.ListConfigs(ctx)
	if err != nil {
		t.Fatalf("ListConfigs: %v", err)
	}
	if configs[0].LastRunStatus == nil || *configs[0].LastRunStatus != RunFailed {
		t.Fatalf("cancelled run should be marked failed: %+v", configs[0])
	}

	retry := NewDiscoverer(s, fakeGitHubClient{})
	retry.RequestInterval = 0
	if _, err := retry.RunConfig(ctx, cfg.ID, TriggerManual, "alice"); err != nil {
		t.Fatalf("RunConfig retry should not be blocked by stale running run: %v", err)
	}
}

func TestDiscovererRunConfigClosesRunWhenUpsertContextIsCanceled(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cfg := DiscoveryConfig{
		ID:          "cfg-cancelled-upsert",
		Name:        "Cancelled upsert",
		Method:      DiscoveryKeyword,
		Terms:       []string{"browser agent"},
		TriggerMode: TriggerManual,
		Enabled:     true,
		CreatedBy:   "alice",
		UpdatedBy:   "alice",
	}
	if err := s.UpsertConfig(ctx, cfg); err != nil {
		t.Fatalf("UpsertConfig: %v", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	d := NewDiscoverer(s, cancelBeforeUpsertGitHubClient{cancel: cancel})
	d.RequestInterval = 0
	_, err := d.RunConfig(runCtx, cfg.ID, TriggerManual, "alice")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunConfig should return the upsert cancellation error, got %v", err)
	}

	configs, err := s.ListConfigs(ctx)
	if err != nil {
		t.Fatalf("ListConfigs: %v", err)
	}
	if configs[0].LastRunStatus == nil || *configs[0].LastRunStatus == RunRunning {
		t.Fatalf("cancelled upsert should not leave a running config: %+v", configs[0])
	}

	retry := NewDiscoverer(s, fakeGitHubClient{})
	retry.RequestInterval = 0
	if _, err := retry.RunConfig(ctx, cfg.ID, TriggerManual, "alice"); err != nil {
		t.Fatalf("RunConfig retry should not be blocked by stale running run: %v", err)
	}
}

func TestDiscovererWaitsBetweenGitHubSearchRequests(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cfg := DiscoveryConfig{
		ID:          "cfg-throttle",
		Name:        "Throttled tools",
		Method:      DiscoveryKeyword,
		Terms:       []string{"browser agent", "research agent"},
		TriggerMode: TriggerManual,
		Enabled:     true,
		CreatedBy:   "alice",
		UpdatedBy:   "alice",
	}
	if err := s.UpsertConfig(ctx, cfg); err != nil {
		t.Fatalf("UpsertConfig: %v", err)
	}

	firstQuery := `"browser agent" in:name,description,topics,readme fork:false archived:false`
	secondQuery := `"research agent" in:name,description,topics,readme fork:false archived:false`
	gh := &recordingGitHubClient{searches: map[string]GitHubSearchResult{
		firstQuery: {
			TotalCount: 1,
			Repos:      []GitHubRepo{sampleRepo("node-browser", "openai", "browser-agent")},
		},
		secondQuery: {
			TotalCount: 1,
			Repos:      []GitHubRepo{sampleRepo("node-research", "openai", "research-agent")},
		},
	}}
	var waits []time.Duration
	d := NewDiscoverer(s, gh)
	d.RequestInterval = 5 * time.Second
	d.Sleep = func(ctx context.Context, duration time.Duration) error {
		waits = append(waits, duration)
		return nil
	}

	result, err := d.RunConfig(ctx, cfg.ID, TriggerManual, "alice")
	if err != nil {
		t.Fatalf("RunConfig: %v", err)
	}
	if result.Status != RunSucceeded || len(gh.calls) != 2 {
		t.Fatalf("run result mismatch: result=%+v calls=%d", result, len(gh.calls))
	}
	if len(waits) != 1 || waits[0] != 5*time.Second {
		t.Fatalf("waits mismatch: %+v", waits)
	}
}

func TestDiscovererRunExistingConfigPausesBeforeNextRequest(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cfg := DiscoveryConfig{
		ID:          "cfg-pause-existing",
		Name:        "Pause existing",
		Method:      DiscoveryKeyword,
		Terms:       []string{"browser agent"},
		TriggerMode: TriggerManual,
		Enabled:     true,
		CreatedBy:   "alice",
		UpdatedBy:   "alice",
	}
	if err := s.UpsertConfig(ctx, cfg); err != nil {
		t.Fatalf("UpsertConfig: %v", err)
	}
	runID := "run-pause-existing"
	if err := s.CreateRun(ctx, DiscoveryRun{
		ID:            runID,
		ConfigID:      &cfg.ID,
		TriggerSource: TriggerManual,
		Actor:         "alice",
		Status:        RunRunning,
		NextPage:      1,
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := s.RequestRunPause(ctx, cfg.ID, "alice"); err != nil {
		t.Fatalf("RequestRunPause: %v", err)
	}

	gh := &recordingGitHubClient{searches: map[string]GitHubSearchResult{}}
	d := NewDiscoverer(s, gh)
	result, err := d.RunExistingConfig(ctx, cfg.ID, runID)
	if err != nil {
		t.Fatalf("RunExistingConfig: %v", err)
	}
	if result.Status != RunPaused || len(gh.calls) != 0 {
		t.Fatalf("paused run should not call GitHub: result=%+v calls=%+v", result, gh.calls)
	}

	active, err := s.GetActiveRun(ctx, cfg.ID)
	if err != nil {
		t.Fatalf("GetActiveRun: %v", err)
	}
	if active.Status != RunPaused || active.NextPage != 1 {
		t.Fatalf("active run should be paused at first page: %+v", active)
	}
}

func TestDiscovererRunExistingConfigResumesFromCheckpoint(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cfg := DiscoveryConfig{
		ID:          "cfg-resume-existing",
		Name:        "Resume existing",
		Method:      DiscoveryKeyword,
		Terms:       []string{"browser agent"},
		TriggerMode: TriggerManual,
		Enabled:     true,
		CreatedBy:   "alice",
		UpdatedBy:   "alice",
	}
	if err := s.UpsertConfig(ctx, cfg); err != nil {
		t.Fatalf("UpsertConfig: %v", err)
	}
	runID := "run-resume-existing"
	if err := s.CreateRun(ctx, DiscoveryRun{
		ID:            runID,
		ConfigID:      &cfg.ID,
		TriggerSource: TriggerManual,
		Actor:         "alice",
		Status:        RunRunning,
		ResultCount:   100,
		PagesScanned:  1,
		NextPage:      2,
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	query := `"browser agent" in:name,description,topics,readme fork:false archived:false`
	gh := &recordingGitHubClient{searches: map[string]GitHubSearchResult{
		query: {
			TotalCount: 101,
			Repos:      []GitHubRepo{sampleRepo("node-resume", "openai", "resume-agent")},
		},
	}}
	d := NewDiscoverer(s, gh)
	d.RequestInterval = 0
	result, err := d.RunExistingConfig(ctx, cfg.ID, runID)
	if err != nil {
		t.Fatalf("RunExistingConfig: %v", err)
	}
	if len(gh.calls) != 1 || gh.calls[0].Page != 2 {
		t.Fatalf("resume should start from page 2, calls=%+v", gh.calls)
	}
	if result.Status != RunSucceeded || result.ResultCount != 101 || result.NextQueryIndex != 1 || result.NextPage != 1 {
		t.Fatalf("resume result mismatch: %+v", result)
	}
}

func TestDiscovererRunConfigRecordsPartialRateLimitWithoutRollingBackSuccess(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cfg := DiscoveryConfig{
		ID:          "cfg-keyword",
		Name:        "Keyword tools",
		Method:      DiscoveryKeyword,
		Terms:       []string{"coding agent", "browser agent"},
		TriggerMode: TriggerManual,
		Enabled:     true,
		CreatedBy:   "alice",
		UpdatedBy:   "alice",
	}
	if err := s.UpsertConfig(ctx, cfg); err != nil {
		t.Fatalf("UpsertConfig: %v", err)
	}

	resetAt := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	limitedQuery := `"browser agent" in:name,description,topics,readme fork:false archived:false`
	gh := fakeGitHubClient{
		searches: map[string]GitHubSearchResult{
			`"coding agent" in:name,description,topics,readme fork:false archived:false`: {
				TotalCount: 1,
				Repos:      []GitHubRepo{sampleRepo("node-ok", "openai", "agents")},
			},
		},
		errors: map[string]error{
			limitedQuery: &DiscoveryError{
				Class:            ErrorRateLimited,
				Message:          "API rate limit exceeded",
				RateLimited:      true,
				RateLimitResetAt: &resetAt,
			},
		},
	}
	d := NewDiscoverer(s, gh)
	d.RequestInterval = 0
	result, err := d.RunConfig(ctx, cfg.ID, TriggerManual, "alice")
	if err != nil {
		t.Fatalf("RunConfig: %v", err)
	}
	if result.Status != RunPartial || !result.RateLimited || result.RateLimitResetAt == nil || result.ErrorClass != ErrorRateLimited {
		t.Fatalf("partial rate limit result mismatch: %+v", result)
	}

	items, err := s.ListTools(ctx, ListToolsParams{Status: ToolDiscovered, Limit: 10})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(items) != 1 || items[0].GitHubNodeID != "node-ok" {
		t.Fatalf("successful result should be retained: %+v", items)
	}
}

func TestDiscovererManualAddRejectsInvalidReposAndDedupesByNodeID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	gh := fakeGitHubClient{repos: map[string]GitHubRepo{
		"openai/agents": sampleRepo("node-1", "openai", "agents"),
		"openai/fork": func() GitHubRepo {
			r := sampleRepo("node-fork", "openai", "fork")
			r.Fork = true
			return r
		}(),
	}}
	d := NewDiscoverer(s, gh)

	first, err := d.ManualAdd(ctx, "https://github.com/openai/agents", "alice")
	if err != nil {
		t.Fatalf("ManualAdd first: %v", err)
	}
	second, err := d.ManualAdd(ctx, "git@github.com:openai/agents.git", "bob")
	if err != nil {
		t.Fatalf("ManualAdd duplicate: %v", err)
	}
	if !first.Created || !second.Duplicate || second.Tool.ID != first.Tool.ID {
		t.Fatalf("manual add dedupe mismatch: first=%+v second=%+v", first, second)
	}

	_, err = d.ManualAdd(ctx, "https://github.com/openai/fork", "alice")
	if err == nil {
		t.Fatal("fork repo should be rejected")
	}
	var derr *DiscoveryError
	if !AsDiscoveryError(err, &derr) || derr.Class != ErrorRepositoryRejected {
		t.Fatalf("fork rejection mismatch: %#v", err)
	}
}

func TestDiscovererRunWeeklyContinuesAfterOneConfigFails(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, cfg := range []DiscoveryConfig{
		{ID: "cfg-ok", Name: "OK", Method: DiscoveryTopic, Terms: []string{"ai-agents"}, TriggerMode: TriggerWeekly, Enabled: true, CreatedBy: "alice", UpdatedBy: "alice"},
		{ID: "cfg-bad", Name: "Bad", Method: DiscoveryTopic, Terms: []string{"bad-query"}, TriggerMode: TriggerWeekly, Enabled: true, CreatedBy: "alice", UpdatedBy: "alice"},
		{ID: "cfg-disabled", Name: "Disabled", Method: DiscoveryTopic, Terms: []string{"disabled"}, TriggerMode: TriggerWeekly, Enabled: false, CreatedBy: "alice", UpdatedBy: "alice"},
	} {
		if err := s.UpsertConfig(ctx, cfg); err != nil {
			t.Fatalf("UpsertConfig %s: %v", cfg.ID, err)
		}
	}

	gh := fakeGitHubClient{
		searches: map[string]GitHubSearchResult{
			"topic:ai-agents fork:false archived:false": {
				TotalCount: 1,
				Repos:      []GitHubRepo{sampleRepo("node-ok", "openai", "agents")},
			},
		},
		errors: map[string]error{
			"topic:bad-query fork:false archived:false": &DiscoveryError{Class: ErrorBadQuery, Message: "validation failed"},
		},
	}
	d := NewDiscoverer(s, gh)
	summary, err := d.RunWeekly(ctx, "cron")
	if err != nil {
		t.Fatalf("RunWeekly: %v", err)
	}
	if summary.Attempted != 2 || summary.Succeeded != 1 || summary.Failed != 1 {
		t.Fatalf("weekly summary mismatch: %+v", summary)
	}

	configs, err := s.ListConfigs(ctx)
	if err != nil {
		t.Fatalf("ListConfigs: %v", err)
	}
	statuses := map[string]RunStatus{}
	for _, cfg := range configs {
		if cfg.LastRunStatus != nil {
			statuses[cfg.ID] = *cfg.LastRunStatus
		}
	}
	if statuses["cfg-ok"] != RunSucceeded || statuses["cfg-bad"] != RunFailed {
		t.Fatalf("weekly config statuses mismatch: %+v", statuses)
	}
	if _, ok := statuses["cfg-disabled"]; ok {
		t.Fatal("disabled config should not run")
	}
}

func TestDiscovererSnapshotActiveToolsBackfillsMissingDailySnapshots(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	upsert, err := s.ManualAdd(ctx, sampleRepo("node-active", "openai", "browser-agent"), "alice")
	if err != nil {
		t.Fatalf("ManualAdd: %v", err)
	}
	if _, err := s.pool.Exec(ctx, "TRUNCATE tool_star_snapshots"); err != nil {
		t.Fatalf("truncate snapshots: %v", err)
	}

	snapshotDay := time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)
	refreshed := sampleRepo("node-active", "openai", "browser-agent")
	refreshed.Stars = 145
	refreshed.Forks = 15
	refreshed.OpenIssues = 5
	d := NewDiscoverer(s, fakeGitHubClient{repos: map[string]GitHubRepo{
		"openai/browser-agent": refreshed,
	}})
	d.RequestInterval = 0
	d.Now = func() time.Time { return snapshotDay }

	summary, err := d.SnapshotActiveTools(ctx, 10)
	if err != nil {
		t.Fatalf("SnapshotActiveTools: %v", err)
	}
	if summary.Attempted != 1 || summary.Saved != 1 || summary.Failed != 0 {
		t.Fatalf("snapshot summary mismatch: %+v", summary)
	}

	var stars int
	var capturedOn time.Time
	if err := s.pool.QueryRow(ctx, `
		SELECT stars, captured_on
		FROM tool_star_snapshots
		WHERE tool_id=$1`, upsert.Tool.ID).Scan(&stars, &capturedOn); err != nil {
		t.Fatalf("snapshot row: %v", err)
	}
	if stars != 145 || !capturedOn.Equal(time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("snapshot mismatch: stars=%d captured_on=%s", stars, capturedOn.Format(time.RFC3339))
	}
}

func TestAsDiscoveryErrorWrapsPlainErrors(t *testing.T) {
	err := errors.New("plain")
	var derr *DiscoveryError
	if AsDiscoveryError(err, &derr) {
		t.Fatal("plain error should not match DiscoveryError")
	}
}
