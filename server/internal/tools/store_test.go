package tools

import (
	"context"
	"testing"
	"time"
)

func TestEnsureSchemaIsIdempotentAndCreatesTables(t *testing.T) {
	s := newTestStore(t)
	if err := s.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("second EnsureSchema: %v", err)
	}

	for _, table := range []string{
		"tool_discovery_configs",
		"tool_discovery_runs",
		"tools",
		"tool_discovery_sources",
		"tool_evaluations",
		"tool_status_events",
		"tool_star_snapshots",
	} {
		t.Run(table, func(t *testing.T) {
			var regclass *string
			if err := s.pool.QueryRow(context.Background(), "SELECT to_regclass($1)::text", "public."+table).Scan(&regclass); err != nil {
				t.Fatalf("to_regclass: %v", err)
			}
			if regclass == nil || *regclass != table {
				t.Fatalf("table not found, got %v", regclass)
			}
		})
	}
}

func TestConfigLifecycleRoundTripsAndSoftDeletes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cfg := DiscoveryConfig{
		ID:          "cfg-keyword",
		Name:        "Coding agents",
		Method:      DiscoveryKeyword,
		Terms:       []string{"coding agent", "agent skills"},
		TriggerMode: TriggerWeekly,
		Enabled:     true,
		CreatedBy:   "alice",
		UpdatedBy:   "alice",
	}
	if err := s.UpsertConfig(ctx, cfg); err != nil {
		t.Fatalf("UpsertConfig: %v", err)
	}

	cfg.Name = "Coding agent tools"
	cfg.Enabled = false
	cfg.UpdatedBy = "bob"
	if err := s.UpsertConfig(ctx, cfg); err != nil {
		t.Fatalf("second UpsertConfig: %v", err)
	}

	configs, err := s.ListConfigs(ctx)
	if err != nil {
		t.Fatalf("ListConfigs: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("want 1 config, got %d", len(configs))
	}
	got := configs[0]
	if got.Name != "Coding agent tools" || got.Enabled || got.UpdatedBy != "bob" {
		t.Fatalf("config not updated: %+v", got)
	}
	if len(got.Terms) != 2 || got.Terms[0] != "coding agent" || got.Terms[1] != "agent skills" {
		t.Fatalf("terms round-trip mismatch: %+v", got.Terms)
	}

	if err := s.SetConfigEnabled(ctx, cfg.ID, true, "carol"); err != nil {
		t.Fatalf("SetConfigEnabled: %v", err)
	}
	configs, err = s.ListConfigs(ctx)
	if err != nil {
		t.Fatalf("ListConfigs after enable: %v", err)
	}
	if !configs[0].Enabled || configs[0].UpdatedBy != "carol" {
		t.Fatalf("enable not applied: %+v", configs[0])
	}

	if err := s.SoftDeleteConfig(ctx, cfg.ID, "dave"); err != nil {
		t.Fatalf("SoftDeleteConfig: %v", err)
	}
	configs, err = s.ListConfigs(ctx)
	if err != nil {
		t.Fatalf("ListConfigs after delete: %v", err)
	}
	if len(configs) != 0 {
		t.Fatalf("soft-deleted config should be hidden, got %+v", configs)
	}
}

func TestUpsertFromGitHubCreatesAndMergesSourcesWithoutStatusRegression(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cfg := DiscoveryConfig{
		ID:          "cfg-topic",
		Name:        "Agent topics",
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

	first, err := s.UpsertFromGitHub(ctx, sampleRepo("node-1", "openai", "agents"), DiscoverySource{
		ConfigID:   &cfg.ID,
		SourceType: SourceTopic,
		Term:       "ai-agents",
		Actor:      "alice",
	})
	if err != nil {
		t.Fatalf("first UpsertFromGitHub: %v", err)
	}
	if !first.Created || first.Tool.Status != ToolDiscovered {
		t.Fatalf("first upsert result mismatch: %+v", first)
	}
	if !containsTag(first.Tool.PurposeTags, "代码开发") || first.Tool.PurposeTagsManuallySet {
		t.Fatalf("first upsert should auto-classify purpose tags: %+v manual=%t", first.Tool.PurposeTags, first.Tool.PurposeTagsManuallySet)
	}

	secondRepo := sampleRepo("node-1", "openai", "agents")
	secondRepo.Stars = 125
	second, err := s.UpsertFromGitHub(ctx, secondRepo, DiscoverySource{
		ConfigID:   &cfg.ID,
		SourceType: SourceKeyword,
		Term:       "coding agent",
		Actor:      "bob",
	})
	if err != nil {
		t.Fatalf("second UpsertFromGitHub: %v", err)
	}
	if second.Created {
		t.Fatalf("second upsert should update existing tool: %+v", second)
	}
	if second.Tool.Status != ToolDiscovered || second.Tool.Stars != 125 {
		t.Fatalf("updated tool mismatch: %+v", second.Tool)
	}

	if _, err := s.pool.Exec(ctx, "UPDATE tools SET status=$1, status_version=status_version+1 WHERE id=$2", ToolEvaluating, first.Tool.ID); err != nil {
		t.Fatalf("force status: %v", err)
	}
	thirdRepo := sampleRepo("node-1", "openai", "agents")
	thirdRepo.Stars = 130
	third, err := s.UpsertFromGitHub(ctx, thirdRepo, DiscoverySource{
		ConfigID:   &cfg.ID,
		SourceType: SourceTopic,
		Term:       "ai-agents",
		Actor:      "carol",
	})
	if err != nil {
		t.Fatalf("third UpsertFromGitHub: %v", err)
	}
	if third.Tool.Status != ToolEvaluating || third.Tool.Stars != 130 {
		t.Fatalf("rediscovery regressed status or missed facts: %+v", third.Tool)
	}

	var toolCount, sourceCount int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM tools WHERE github_node_id='node-1'").Scan(&toolCount); err != nil {
		t.Fatalf("tool count: %v", err)
	}
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM tool_discovery_sources WHERE tool_id=$1", first.Tool.ID).Scan(&sourceCount); err != nil {
		t.Fatalf("source count: %v", err)
	}
	if toolCount != 1 || sourceCount != 2 {
		t.Fatalf("dedupe/source merge mismatch: tools=%d sources=%d", toolCount, sourceCount)
	}
	filtered, err := s.ListTools(ctx, ListToolsParams{
		Status:      ToolDiscovered,
		SourceTypes: []SourceType{SourceKeyword, SourceTopic},
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("ListTools by multiple sources: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != first.Tool.ID {
		t.Fatalf("multi-source filter should include merged tool: %+v", filtered)
	}
	manualOnly, err := s.ListTools(ctx, ListToolsParams{
		Status:      ToolDiscovered,
		SourceTypes: []SourceType{SourceManual},
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("ListTools by manual source: %v", err)
	}
	if len(manualOnly) != 0 {
		t.Fatalf("manual-only filter should exclude keyword/topic tool: %+v", manualOnly)
	}

	var snapshotCount, snapshotStars int
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*), COALESCE(max(stars), 0)
		FROM tool_star_snapshots
		WHERE tool_id=$1`, first.Tool.ID).Scan(&snapshotCount, &snapshotStars); err != nil {
		t.Fatalf("snapshot stats: %v", err)
	}
	if snapshotCount != 1 || snapshotStars != 130 {
		t.Fatalf("daily snapshot should be upserted with latest facts, count=%d stars=%d", snapshotCount, snapshotStars)
	}
}

func TestToolPurposeTagsCanBeManuallyCorrectedAndFiltered(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	repo := sampleRepo("node-purpose", "openai", "browser-agent")
	repo.Description = "A browser automation agent for Playwright tasks"
	repo.Topics = []string{"browser-automation", "ai-agents"}
	upsert, err := s.UpsertFromGitHub(ctx, repo, DiscoverySource{
		SourceType: SourceKeyword,
		Term:       "browser agent",
		Actor:      "alice",
	})
	if err != nil {
		t.Fatalf("UpsertFromGitHub: %v", err)
	}
	if !containsTag(upsert.Tool.PurposeTags, "浏览器操作") {
		t.Fatalf("tool should be auto-classified as browser scenario: %+v", upsert.Tool.PurposeTags)
	}

	if err := s.UpdateToolPurposeTags(ctx, upsert.Tool.ID, []string{"工作流自动化"}, true); err != nil {
		t.Fatalf("UpdateToolPurposeTags: %v", err)
	}
	repo.Description = "A code review agent"
	rediscovered, err := s.UpsertFromGitHub(ctx, repo, DiscoverySource{
		SourceType: SourceTopic,
		Term:       "ai-agents",
		Actor:      "cron",
	})
	if err != nil {
		t.Fatalf("rediscover: %v", err)
	}
	if !rediscovered.Tool.PurposeTagsManuallySet || len(rediscovered.Tool.PurposeTags) != 1 || rediscovered.Tool.PurposeTags[0] != "工作流自动化" {
		t.Fatalf("manual purpose tags should not be overwritten: %+v manual=%t", rediscovered.Tool.PurposeTags, rediscovered.Tool.PurposeTagsManuallySet)
	}

	filtered, err := s.ListTools(ctx, ListToolsParams{
		Status:      ToolDiscovered,
		PurposeTags: []string{"工作流自动化"},
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("ListTools by purpose: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != upsert.Tool.ID {
		t.Fatalf("purpose filter mismatch: %+v", filtered)
	}
	stats, err := s.ToolStats(ctx, ListToolsParams{Status: ToolDiscovered})
	if err != nil {
		t.Fatalf("ToolStats: %v", err)
	}
	if !containsTag(stats.PurposeTags, "工作流自动化") {
		t.Fatalf("stats should expose purpose options: %+v", stats.PurposeTags)
	}
}

func TestRunsAndStarSnapshotsRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cfg := DiscoveryConfig{
		ID:          "cfg-weekly",
		Name:        "Weekly agents",
		Method:      DiscoveryKeyword,
		Terms:       []string{"coding agent"},
		TriggerMode: TriggerWeekly,
		Enabled:     true,
		CreatedBy:   "alice",
		UpdatedBy:   "alice",
	}
	if err := s.UpsertConfig(ctx, cfg); err != nil {
		t.Fatalf("UpsertConfig: %v", err)
	}
	run := DiscoveryRun{
		ID:            "run-1",
		ConfigID:      &cfg.ID,
		TriggerSource: TriggerWeekly,
		Actor:         "cron",
		Status:        RunRunning,
	}
	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	finished := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	if err := s.FinishRun(ctx, RunResult{
		ID:                run.ID,
		Status:            RunPartial,
		FinishedAt:        &finished,
		ResultCount:       3,
		NewCount:          1,
		UpdatedCount:      2,
		SkippedCount:      0,
		PagesScanned:      5,
		IncompleteResults: true,
		Truncated:         true,
		ErrorClass:        "rate_limited",
		ErrorMessage:      "GitHub API rate limit exceeded",
	}); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}

	configs, err := s.ListConfigs(ctx)
	if err != nil {
		t.Fatalf("ListConfigs: %v", err)
	}
	if len(configs) != 1 || configs[0].LastResultCount != 3 || configs[0].LastFailureReason == nil {
		t.Fatalf("config run info not updated: %+v", configs)
	}

	upsert, err := s.UpsertFromGitHub(ctx, sampleRepo("node-2", "anthropic", "agent-sdk"), DiscoverySource{
		ConfigID:   &cfg.ID,
		SourceType: SourceKeyword,
		Term:       "coding agent",
		Actor:      "cron",
	})
	if err != nil {
		t.Fatalf("UpsertFromGitHub: %v", err)
	}
	if _, err := s.pool.Exec(ctx, "DELETE FROM tool_star_snapshots WHERE tool_id=$1", upsert.Tool.ID); err != nil {
		t.Fatalf("clear automatic snapshot: %v", err)
	}
	oldDay := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	newDay := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	if err := s.SaveStarSnapshot(ctx, StarSnapshot{
		ID:         "snap-old",
		ToolID:     upsert.Tool.ID,
		Stars:      90,
		Forks:      8,
		OpenIssues: 2,
		SnapshotAt: oldDay,
	}); err != nil {
		t.Fatalf("SaveStarSnapshot old: %v", err)
	}
	if err := s.SaveStarSnapshot(ctx, StarSnapshot{
		ID:         "snap-new",
		ToolID:     upsert.Tool.ID,
		Stars:      125,
		Forks:      10,
		OpenIssues: 3,
		SnapshotAt: newDay,
	}); err != nil {
		t.Fatalf("SaveStarSnapshot new: %v", err)
	}

	items, err := s.ListTools(ctx, ListToolsParams{
		Status: ToolDiscovered,
		Sort:   SortStars7D,
		Now:    newDay,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 tool, got %d", len(items))
	}
	if items[0].Stars != 125 {
		t.Fatalf("current stars should refresh from latest snapshot, got %d", items[0].Stars)
	}
	if items[0].Stars7D == nil || *items[0].Stars7D != 35 {
		t.Fatalf("stars delta mismatch: %+v", items[0].Stars7D)
	}
}

func TestRunPauseResumeLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cfg := DiscoveryConfig{
		ID:          "cfg-pause",
		Name:        "Pausable discovery",
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
	if err := s.CreateRun(ctx, DiscoveryRun{
		ID:            "run-pause",
		ConfigID:      &cfg.ID,
		TriggerSource: TriggerManual,
		Actor:         "alice",
		Status:        RunRunning,
		NextPage:      1,
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if err := s.UpdateRunProgress(ctx, RunResult{
		ID:             "run-pause",
		ResultCount:    50,
		NewCount:       20,
		UpdatedCount:   30,
		PagesScanned:   2,
		NextQueryIndex: 0,
		NextPage:       3,
	}); err != nil {
		t.Fatalf("UpdateRunProgress: %v", err)
	}
	requested, err := s.RequestRunPause(ctx, cfg.ID, "bob")
	if err != nil {
		t.Fatalf("RequestRunPause: %v", err)
	}
	if !requested.PauseRequested || requested.Status != RunRunning || requested.NextPage != 3 {
		t.Fatalf("pause request mismatch: %+v", requested)
	}
	configs, err := s.ListConfigs(ctx)
	if err != nil {
		t.Fatalf("ListConfigs after pause request: %v", err)
	}
	if !configs[0].LastPauseRequested || configs[0].LastPagesScanned != 2 {
		t.Fatalf("config should expose pending pause and progress: %+v", configs[0])
	}

	if err := s.PauseRun(ctx, RunResult{
		ID:             "run-pause",
		ResultCount:    50,
		NewCount:       20,
		UpdatedCount:   30,
		PagesScanned:   2,
		NextQueryIndex: 0,
		NextPage:       3,
	}); err != nil {
		t.Fatalf("PauseRun: %v", err)
	}
	paused, err := s.GetActiveRun(ctx, cfg.ID)
	if err != nil {
		t.Fatalf("GetActiveRun paused: %v", err)
	}
	if paused.Status != RunPaused || paused.PauseRequested || paused.NextPage != 3 {
		t.Fatalf("paused run mismatch: %+v", paused)
	}
	if err := s.CreateRun(ctx, DiscoveryRun{
		ID:            "run-blocked-by-paused",
		ConfigID:      &cfg.ID,
		TriggerSource: TriggerManual,
		Actor:         "alice",
		Status:        RunRunning,
	}); err == nil {
		t.Fatal("paused run should still block a second active run")
	}

	resumed, err := s.ResumeRun(ctx, cfg.ID, "carol")
	if err != nil {
		t.Fatalf("ResumeRun: %v", err)
	}
	if resumed.Status != RunRunning || resumed.Actor != "carol" || resumed.NextPage != 3 {
		t.Fatalf("resumed run mismatch: %+v", resumed)
	}
}

func TestListToolsUsesEarliestRecentSnapshotWhenSevenDayBaselineMissing(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	upsert, err := s.ManualAdd(ctx, sampleRepo("node-recent", "openai", "recent-agent"), "alice")
	if err != nil {
		t.Fatalf("ManualAdd: %v", err)
	}
	if _, err := s.pool.Exec(ctx, "DELETE FROM tool_star_snapshots WHERE tool_id=$1", upsert.Tool.ID); err != nil {
		t.Fatalf("clear automatic snapshot: %v", err)
	}

	oldDay := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	newDay := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	if err := s.SaveStarSnapshot(ctx, StarSnapshot{
		ID:         "snap-recent-old",
		ToolID:     upsert.Tool.ID,
		Stars:      100,
		Forks:      8,
		OpenIssues: 2,
		SnapshotAt: oldDay,
	}); err != nil {
		t.Fatalf("SaveStarSnapshot old: %v", err)
	}
	if err := s.SaveStarSnapshot(ctx, StarSnapshot{
		ID:         "snap-recent-new",
		ToolID:     upsert.Tool.ID,
		Stars:      116,
		Forks:      10,
		OpenIssues: 3,
		SnapshotAt: newDay,
	}); err != nil {
		t.Fatalf("SaveStarSnapshot new: %v", err)
	}

	items, err := s.ListTools(ctx, ListToolsParams{
		Status: ToolDiscovered,
		Sort:   SortStars7D,
		Now:    newDay,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 tool, got %d", len(items))
	}
	if items[0].Stars7D == nil || *items[0].Stars7D != 16 {
		t.Fatalf("recent stars delta mismatch: %+v", items[0].Stars7D)
	}
}

func TestListToolsAppliesOffsetAfterSorting(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	high := sampleRepo("node-page-high", "openai", "page-high")
	high.Stars = 300
	mid := sampleRepo("node-page-mid", "openai", "page-mid")
	mid.Stars = 200
	low := sampleRepo("node-page-low", "openai", "page-low")
	low.Stars = 100
	for _, repo := range []GitHubRepo{high, mid, low} {
		if _, err := s.ManualAdd(ctx, repo, "alice"); err != nil {
			t.Fatalf("ManualAdd %s: %v", repo.FullName, err)
		}
	}

	items, err := s.ListTools(ctx, ListToolsParams{
		Status: ToolDiscovered,
		Sort:   SortStars,
		Limit:  1,
		Offset: 1,
	})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 paged tool, got %d", len(items))
	}
	if items[0].GitHubFullName != "openai/page-mid" {
		t.Fatalf("offset should apply after star sorting, got %s", items[0].GitHubFullName)
	}
}

func TestCreateRunRejectsConcurrentConfigRun(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cfg := DiscoveryConfig{
		ID:          "cfg-concurrent",
		Name:        "Concurrent runs",
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

	if err := s.CreateRun(ctx, DiscoveryRun{
		ID:            "run-active",
		ConfigID:      &cfg.ID,
		TriggerSource: TriggerManual,
		Actor:         "alice",
		Status:        RunRunning,
	}); err != nil {
		t.Fatalf("CreateRun first: %v", err)
	}
	err := s.CreateRun(ctx, DiscoveryRun{
		ID:            "run-duplicate",
		ConfigID:      &cfg.ID,
		TriggerSource: TriggerManual,
		Actor:         "alice",
		Status:        RunRunning,
	})
	var derr *DiscoveryError
	if !AsDiscoveryError(err, &derr) || derr.Class != ErrorStatusConflict {
		t.Fatalf("duplicate run should be status conflict, got %#v", err)
	}

	finishedAt := time.Now().UTC()
	if err := s.FinishRun(ctx, RunResult{ID: "run-active", Status: RunSucceeded, FinishedAt: &finishedAt}); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}
	if err := s.CreateRun(ctx, DiscoveryRun{
		ID:            "run-after-finish",
		ConfigID:      &cfg.ID,
		TriggerSource: TriggerManual,
		Actor:         "alice",
		Status:        RunRunning,
	}); err != nil {
		t.Fatalf("CreateRun after finish: %v", err)
	}
}
