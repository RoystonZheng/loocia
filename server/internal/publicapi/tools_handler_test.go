package publicapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"aihot-server/internal/db"
	"aihot-server/internal/tools"

	"github.com/jackc/pgx/v5"
)

func waitForNoActiveRun(t *testing.T, store *tools.Store, configID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, err := store.GetActiveRun(context.Background(), configID)
		if errors.Is(err, pgx.ErrNoRows) {
			return
		}
		if err != nil {
			t.Fatalf("GetActiveRun: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("active run did not finish for config %s", configID)
}

type toolEnvelopeForTest[T any] struct {
	Errno  int    `json:"errno"`
	Errmsg string `json:"errmsg"`
	Data   T      `json:"data"`
}

type fakeToolGitHub struct {
	repos map[string]tools.GitHubRepo
}

type fakeToolSummaryGenerator struct {
	summary string
	err     error
	calls   int
}

func (f fakeToolGitHub) SearchRepositories(context.Context, tools.GitHubSearchRequest) (tools.GitHubSearchResult, error) {
	return tools.GitHubSearchResult{}, nil
}

func (f fakeToolGitHub) GetRepository(_ context.Context, owner, repo string) (tools.GitHubRepo, error) {
	if got, ok := f.repos[owner+"/"+repo]; ok {
		return got, nil
	}
	return tools.GitHubRepo{}, &tools.DiscoveryError{Class: tools.ErrorGitHub, Message: "not found"}
}

func (f *fakeToolSummaryGenerator) GenerateToolSummary(context.Context, ToolSummaryInput) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	return f.summary, nil
}

func liveToolAPIStore(t *testing.T) *tools.Store {
	t.Helper()
	dsn := os.Getenv("AI_TOOL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AI_TOOL_TEST_DATABASE_URL not set")
	}
	pool, err := db.NewPool(context.Background(), dsn)
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	t.Cleanup(pool.Close)
	s := tools.New(pool)
	if err := s.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `
		TRUNCATE tool_status_events, tool_star_snapshots, tool_evaluations,
			tool_discovery_sources, tool_discovery_runs, tools,
			tool_discovery_configs CASCADE`); err != nil {
		t.Fatalf("truncate tools: %v", err)
	}
	return s
}

func newToolAPITestHandler(store *tools.Store, repos map[string]tools.GitHubRepo) *ToolAPIHandler {
	return NewToolAPIHandler(store, tools.NewDiscoverer(store, fakeToolGitHub{repos: repos}))
}

func doToolAPI[T any](t *testing.T, h http.Handler, method, path string, body any, wantCode int) toolEnvelopeForTest[T] {
	t.Helper()
	var reqBody *bytes.Reader
	if body == nil {
		reqBody = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reqBody = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != wantCode {
		t.Fatalf("%s %s code=%d body=%s", method, path, rr.Code, rr.Body.String())
	}
	var env toolEnvelopeForTest[T]
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rr.Body.String())
	}
	return env
}

func sampleToolRepo(nodeID, owner, repo string) tools.GitHubRepo {
	pushed := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	return tools.GitHubRepo{
		NodeID:        nodeID,
		Owner:         owner,
		Repo:          repo,
		FullName:      owner + "/" + repo,
		URL:           "https://github.com/" + owner + "/" + repo,
		HomepageURL:   "https://example.com/" + repo,
		Name:          repo,
		Description:   "A browser automation agent",
		Summary:       "用于浏览器自动化的 AI agent 工具",
		Stars:         321,
		Forks:         12,
		OpenIssues:    4,
		Topics:        []string{"browser-agent", "ai-agents"},
		LicenseSPDX:   "MIT",
		DefaultBranch: "main",
		PushedAt:      &pushed,
	}
}

func TestToolAPIConfigLifecycle(t *testing.T) {
	store := liveToolAPIStore(t)
	h := newToolAPITestHandler(store, nil)

	type configData struct {
		Config configJSON `json:"config"`
	}
	created := doToolAPI[configData](t, h, http.MethodPost, "/api/tools/configs", map[string]any{
		"name":        "浏览器操作工具发现",
		"method":      "keyword",
		"terms":       []string{"browser agent", "browser agent", "computer use"},
		"triggerMode": "weekly",
		"enabled":     true,
		"actor":       "alice",
	}, http.StatusOK)
	if created.Errno != 0 || created.Data.Config.ID == "" {
		t.Fatalf("create config response mismatch: %+v", created)
	}
	if got := created.Data.Config.Terms; len(got) != 2 || got[0] != "browser agent" || got[1] != "computer use" {
		t.Fatalf("terms should be deduped and ordered: %+v", got)
	}

	listed := doToolAPI[configListEnvelope](t, h, http.MethodGet, "/api/tools/configs?q=browser", nil, http.StatusOK)
	if listed.Data.Count != 1 || listed.Data.EnabledCount != 1 || listed.Data.DisabledCount != 0 {
		t.Fatalf("list stats mismatch: %+v", listed.Data)
	}
	filtered := doToolAPI[configListEnvelope](t, h, http.MethodGet, "/api/tools/configs?q=browser&method=keyword&triggerMode=weekly", nil, http.StatusOK)
	if filtered.Data.Count != 1 || filtered.Data.Items[0].ID != created.Data.Config.ID {
		t.Fatalf("filtered config list mismatch: %+v", filtered.Data)
	}
	filteredOut := doToolAPI[configListEnvelope](t, h, http.MethodGet, "/api/tools/configs?method=topic&triggerMode=weekly", nil, http.StatusOK)
	if filteredOut.Data.Count != 0 {
		t.Fatalf("config method filter should exclude keyword config: %+v", filteredOut.Data)
	}

	updated := doToolAPI[configData](t, h, http.MethodPost, "/api/tools/configs", map[string]any{
		"id":          created.Data.Config.ID,
		"name":        "浏览器操作工具发现更新",
		"method":      "keyword",
		"terms":       []string{"browser agent"},
		"triggerMode": "weekly",
		"enabled":     true,
	}, http.StatusOK)
	if updated.Data.Config.UpdatedBy != "alice" {
		t.Fatalf("missing actor should reuse existing operator: %+v", updated.Data.Config)
	}

	disabled := doToolAPI[configData](t, h, http.MethodPost, "/api/tools/configs/enable", map[string]any{
		"id":      created.Data.Config.ID,
		"enabled": false,
	}, http.StatusOK)
	if disabled.Data.Config.Enabled || disabled.Data.Config.UpdatedBy != "alice" {
		t.Fatalf("config should be disabled: %+v", disabled.Data.Config)
	}

	type runData struct {
		Run runJSON `json:"run"`
	}
	ran := doToolAPI[runData](t, h, http.MethodPost, "/api/tools/configs/run", map[string]any{
		"id": created.Data.Config.ID,
	}, http.StatusOK)
	if ran.Data.Run.Status != string(tools.RunRunning) || ran.Data.Run.NextPage != 1 {
		t.Fatalf("run should start asynchronously with existing operator: %+v", ran.Data.Run)
	}

	configID := created.Data.Config.ID
	waitForNoActiveRun(t, store, configID)

	if err := store.CreateRun(context.Background(), tools.DiscoveryRun{
		ID:            "run-pause-api-test",
		ConfigID:      &configID,
		TriggerSource: tools.TriggerManual,
		Actor:         "alice",
		Status:        tools.RunRunning,
		PagesScanned:  1,
		NextPage:      2,
	}); err != nil {
		t.Fatalf("CreateRun pause test: %v", err)
	}
	pause := doToolAPI[runData](t, h, http.MethodPost, "/api/tools/configs/pause", map[string]any{
		"id":    created.Data.Config.ID,
		"actor": "alice",
	}, http.StatusOK)
	if pause.Data.Run.Status != string(tools.RunRunning) || !pause.Data.Run.PauseRequested {
		t.Fatalf("pause response mismatch: %+v", pause.Data.Run)
	}
	if err := store.PauseRun(context.Background(), tools.RunResult{
		ID:             "run-pause-api-test",
		PagesScanned:   1,
		NextQueryIndex: 0,
		NextPage:       2,
	}); err != nil {
		t.Fatalf("PauseRun test setup: %v", err)
	}
	resume := doToolAPI[runData](t, h, http.MethodPost, "/api/tools/configs/resume", map[string]any{
		"id":    created.Data.Config.ID,
		"actor": "alice",
	}, http.StatusOK)
	if resume.Data.Run.Status != string(tools.RunRunning) || resume.Data.Run.NextPage != 2 {
		t.Fatalf("resume response mismatch: %+v", resume.Data.Run)
	}
	waitForNoActiveRun(t, store, configID)

	if err := store.CreateRun(context.Background(), tools.DiscoveryRun{
		ID:            "run-running-api-test",
		ConfigID:      &configID,
		TriggerSource: tools.TriggerManual,
		Actor:         "alice",
		Status:        tools.RunRunning,
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	conflict := doToolAPI[toolAPIErrorData](t, h, http.MethodPost, "/api/tools/configs/delete", map[string]any{
		"id":    created.Data.Config.ID,
		"actor": "alice",
	}, http.StatusConflict)
	if conflict.Errno != 409001 || conflict.Data.Class != tools.ErrorStatusConflict {
		t.Fatalf("delete conflict mismatch: %+v", conflict)
	}

	finishedAt := time.Now().UTC()
	if err := store.FinishRun(context.Background(), tools.RunResult{
		ID:         "run-running-api-test",
		Status:     tools.RunFailed,
		FinishedAt: &finishedAt,
	}); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}
	deleted := doToolAPI[map[string]bool](t, h, http.MethodPost, "/api/tools/configs/delete", map[string]any{
		"id": created.Data.Config.ID,
	}, http.StatusOK)
	if !deleted.Data["deleted"] {
		t.Fatalf("delete response mismatch: %+v", deleted)
	}
	listedAfterDelete := doToolAPI[configListEnvelope](t, h, http.MethodGet, "/api/tools/configs", nil, http.StatusOK)
	if listedAfterDelete.Data.Count != 0 {
		t.Fatalf("deleted config should be hidden: %+v", listedAfterDelete.Data)
	}
}

func TestToolAPIManualAddAndEvaluationClosure(t *testing.T) {
	store := liveToolAPIStore(t)
	repo := sampleToolRepo("node-browser-agent", "openai", "browser-agent")
	h := newToolAPITestHandler(store, map[string]tools.GitHubRepo{repo.FullName: repo})

	type previewData struct {
		Repository githubRepoJSON `json:"repository"`
	}
	preview := doToolAPI[previewData](t, h, http.MethodPost, "/api/tools/manual/preview", map[string]any{
		"repoUrl": "https://github.com/openai/browser-agent",
	}, http.StatusOK)
	if preview.Data.Repository.NodeID != repo.NodeID || preview.Data.Repository.FullName != repo.FullName {
		t.Fatalf("preview mismatch: %+v", preview.Data.Repository)
	}
	if len(preview.Data.Repository.PurposeTags) == 0 || preview.Data.Repository.PurposeTags[0] != "浏览器操作" {
		t.Fatalf("preview should include auto purpose tags: %+v", preview.Data.Repository.PurposeTags)
	}

	type manualData struct {
		Tool      toolItemJSON `json:"tool"`
		Created   bool         `json:"created"`
		Duplicate bool         `json:"duplicate"`
	}
	added := doToolAPI[manualData](t, h, http.MethodPost, "/api/tools/manual/add", map[string]any{
		"repoUrl":     "https://github.com/openai/browser-agent",
		"actor":       "alice",
		"purposeTags": []string{"浏览器操作"},
	}, http.StatusOK)
	if !added.Data.Created || added.Data.Duplicate || added.Data.Tool.Status != string(tools.ToolDiscovered) {
		t.Fatalf("manual add mismatch: %+v", added.Data)
	}
	if !added.Data.Tool.PurposeTagsManuallySet || len(added.Data.Tool.PurposeTags) != 1 || added.Data.Tool.PurposeTags[0] != "浏览器操作" {
		t.Fatalf("manual purpose tags mismatch: %+v", added.Data.Tool)
	}

	discovered := doToolAPI[toolListEnvelope](t, h, http.MethodGet, "/api/tools/items?status=discovered&source=manual&purposeTag=%E6%B5%8F%E8%A7%88%E5%99%A8%E6%93%8D%E4%BD%9C", nil, http.StatusOK)
	if discovered.Data.Count != 1 || discovered.Data.Stats.ManualSourceCount != 1 || len(discovered.Data.Stats.PurposeTags) == 0 {
		t.Fatalf("discovered list mismatch: %+v", discovered.Data)
	}

	missingActor := doToolAPI[toolAPIErrorData](t, h, http.MethodPost, "/api/tools/purpose-tags", map[string]any{
		"toolId":      added.Data.Tool.ID,
		"purposeTags": []string{"工作流自动化"},
	}, http.StatusBadRequest)
	if missingActor.Data.Class != tools.ErrorValidation {
		t.Fatalf("missing actor purpose update mismatch: %+v", missingActor)
	}
	updatedPurpose := doToolAPI[manualData](t, h, http.MethodPost, "/api/tools/purpose-tags", map[string]any{
		"toolId":      added.Data.Tool.ID,
		"actor":       "alice",
		"purposeTags": []string{"工作流自动化"},
	}, http.StatusOK)
	if len(updatedPurpose.Data.Tool.PurposeTags) != 1 || updatedPurpose.Data.Tool.PurposeTags[0] != "工作流自动化" {
		t.Fatalf("purpose update mismatch: %+v", updatedPurpose.Data.Tool.PurposeTags)
	}

	type evaluationData struct {
		Evaluation evaluationSummaryJSON `json:"evaluation"`
	}
	started := doToolAPI[evaluationData](t, h, http.MethodPost, "/api/tools/evaluations/start", map[string]any{
		"toolId":    added.Data.Tool.ID,
		"evaluator": "张三",
		"operator":  "李四",
	}, http.StatusOK)
	if started.Data.Evaluation.ID == "" || started.Data.Evaluation.CooperURL != nil {
		t.Fatalf("start evaluation mismatch: %+v", started.Data.Evaluation)
	}

	missingURL := doToolAPI[toolAPIErrorData](t, h, http.MethodPost, "/api/tools/evaluations/finish", map[string]any{
		"toolId":       added.Data.Tool.ID,
		"evaluationId": started.Data.Evaluation.ID,
		"result":       "included",
		"operator":     "李四",
		"finalSummary": "这是一个面向研发团队的浏览器自动化 AI agent 工具，适合用于网页任务验证和操作流程辅助。",
	}, http.StatusBadRequest)
	if missingURL.Data.Class != tools.ErrorMissingCooperURL {
		t.Fatalf("missing cooper error mismatch: %+v", missingURL)
	}

	linked := doToolAPI[evaluationData](t, h, http.MethodPost, "/api/tools/evaluations/cooper-url", map[string]any{
		"toolId":    added.Data.Tool.ID,
		"cooperUrl": "https://cooper.didichuxing.com/didocs/2209600482571",
		"operator":  "李四",
	}, http.StatusOK)
	if linked.Data.Evaluation.CooperURL == nil || *linked.Data.Evaluation.CooperURL != "https://cooper.didichuxing.com/didocs/2209600482571" {
		t.Fatalf("linked evaluation mismatch: %+v", linked.Data.Evaluation)
	}

	longSummary := strings.Repeat("这是一个面向团队的研究工具，", 35)
	completed := doToolAPI[map[string]bool](t, h, http.MethodPost, "/api/tools/evaluations/finish", map[string]any{
		"toolId":       added.Data.Tool.ID,
		"evaluationId": started.Data.Evaluation.ID,
		"result":       "included",
		"operator":     "李四",
		"finalSummary": longSummary,
	}, http.StatusOK)
	if !completed.Data["completed"] {
		t.Fatalf("finish response mismatch: %+v", completed)
	}

	included := doToolAPI[toolListEnvelope](t, h, http.MethodGet, "/api/tools/items?status=included&q=张三", nil, http.StatusOK)
	if included.Data.Count != 1 {
		t.Fatalf("included tool should be searchable by evaluator: %+v", included.Data)
	}
	item := included.Data.Items[0]
	if item.Evaluation == nil || item.Evaluation.Evaluator != "张三" || item.CurrentSummary == nil {
		t.Fatalf("included evaluation projection mismatch: %+v", item)
	}
	if item.CurrentSummarySource != "final" {
		t.Fatalf("included list should use final summary, got %q", item.CurrentSummarySource)
	}
}

func TestToolAPIToolSummaryByURLKey(t *testing.T) {
	store := liveToolAPIStore(t)
	repo := sampleToolRepo("node-summary-tool", "openai", "summary-tool")
	repo.Summary = ""
	generator := &fakeToolSummaryGenerator{summary: "用于整理浏览器自动化任务的团队工具。"}
	h := newToolAPITestHandler(store, map[string]tools.GitHubRepo{repo.FullName: repo}).
		WithSummaryGenerator(generator)

	type manualData struct {
		Tool      toolItemJSON `json:"tool"`
		Created   bool         `json:"created"`
		Duplicate bool         `json:"duplicate"`
	}
	added := doToolAPI[manualData](t, h, http.MethodPost, "/api/tools/manual/add", map[string]any{
		"repoUrl": "https://github.com/openai/summary-tool",
		"actor":   "alice",
	}, http.StatusOK)
	if added.Data.Tool.SummaryKey == "" {
		t.Fatalf("summary key should be projected: %+v", added.Data.Tool)
	}

	generated := doToolAPI[toolSummaryResponse](t, h, http.MethodPost, "/api/tools/summaries/url-key", map[string]any{
		"toolId": added.Data.Tool.ID,
		"urlKey": added.Data.Tool.SummaryKey,
	}, http.StatusOK)
	if generated.Data.Summary != generator.summary || !generated.Data.Translated || generator.calls != 1 {
		t.Fatalf("generated summary mismatch: response=%+v calls=%d", generated.Data, generator.calls)
	}

	listed := doToolAPI[toolListEnvelope](t, h, http.MethodGet, "/api/tools/items?status=discovered&q=summary-tool", nil, http.StatusOK)
	if listed.Data.Count != 1 || listed.Data.Items[0].CurrentSummary == nil || *listed.Data.Items[0].CurrentSummary != generator.summary {
		t.Fatalf("generated summary should be cached in list response: %+v", listed.Data)
	}
	if listed.Data.Items[0].CurrentSummarySource != "temporary" {
		t.Fatalf("cached summary should use temporary source: %+v", listed.Data.Items[0])
	}

	badKey := doToolAPI[toolAPIErrorData](t, h, http.MethodPost, "/api/tools/summaries/url-key", map[string]any{
		"toolId": added.Data.Tool.ID,
		"urlKey": "bad-key",
	}, http.StatusBadRequest)
	if badKey.Data.Class != tools.ErrorValidation {
		t.Fatalf("bad summary key error mismatch: %+v", badKey)
	}
}

func TestToolAPIToolSummaryByURLKeyFallsBackWhenGeneratorFails(t *testing.T) {
	store := liveToolAPIStore(t)
	repo := sampleToolRepo("node-summary-fallback-tool", "openai", "summary-fallback-tool")
	repo.Summary = ""
	generator := &fakeToolSummaryGenerator{err: errors.New("summary timeout")}
	h := newToolAPITestHandler(store, map[string]tools.GitHubRepo{repo.FullName: repo}).
		WithSummaryGenerator(generator)

	type manualData struct {
		Tool      toolItemJSON `json:"tool"`
		Created   bool         `json:"created"`
		Duplicate bool         `json:"duplicate"`
	}
	added := doToolAPI[manualData](t, h, http.MethodPost, "/api/tools/manual/add", map[string]any{
		"repoUrl": "https://github.com/openai/summary-fallback-tool",
		"actor":   "alice",
	}, http.StatusOK)

	generated := doToolAPI[toolSummaryResponse](t, h, http.MethodPost, "/api/tools/summaries/url-key", map[string]any{
		"toolId": added.Data.Tool.ID,
		"urlKey": added.Data.Tool.SummaryKey,
	}, http.StatusOK)
	if generated.Data.Summary == "" || !hasHan(generated.Data.Summary) || !generated.Data.Translated {
		t.Fatalf("fallback summary should be usable Chinese: %+v", generated.Data)
	}
	if !strings.Contains(generated.Data.Summary, "summary-fallback-tool") {
		t.Fatalf("fallback summary should identify the tool: %+v", generated.Data)
	}
	if !strings.HasPrefix(generated.Data.Source, "url_key_fallback:") || generator.calls != 1 {
		t.Fatalf("fallback summary source/calls mismatch: response=%+v calls=%d", generated.Data, generator.calls)
	}

	listed := doToolAPI[toolListEnvelope](t, h, http.MethodGet, "/api/tools/items?status=discovered&q=summary-fallback-tool", nil, http.StatusOK)
	if listed.Data.Count != 1 || listed.Data.Items[0].CurrentSummary == nil || !hasHan(*listed.Data.Items[0].CurrentSummary) {
		t.Fatalf("fallback summary should be cached in list response: %+v", listed.Data)
	}
}

func TestToolAPITeamToolCRUDAndListCount(t *testing.T) {
	store := liveToolAPIStore(t)
	repo := sampleToolRepo("node-team-tool", "openai", "team-tool")
	h := newToolAPITestHandler(store, map[string]tools.GitHubRepo{repo.FullName: repo})

	type manualData struct {
		Tool      toolItemJSON `json:"tool"`
		Created   bool         `json:"created"`
		Duplicate bool         `json:"duplicate"`
	}
	missingOperator := doToolAPI[toolAPIErrorData](t, h, http.MethodPost, "/api/tools/team/import", map[string]any{
		"repoUrl": "https://github.com/openai/team-tool",
	}, http.StatusBadRequest)
	if missingOperator.Data.Class != tools.ErrorValidation {
		t.Fatalf("missing operator error mismatch: %+v", missingOperator)
	}
	imported := doToolAPI[manualData](t, h, http.MethodPost, "/api/tools/team/import", map[string]any{
		"repoUrl":      "https://github.com/openai/team-tool",
		"operator":     "alice",
		"finalSummary": "团队用它处理浏览器任务。",
	}, http.StatusOK)
	if !imported.Data.Created || imported.Data.Tool.Status != string(tools.ToolIncluded) {
		t.Fatalf("team import mismatch: %+v", imported.Data)
	}

	type updateData struct {
		Updated bool `json:"updated"`
	}
	updated := doToolAPI[updateData](t, h, http.MethodPost, "/api/tools/team/update", map[string]any{
		"toolId":       imported.Data.Tool.ID,
		"operator":     "bob",
		"cooperUrl":    "https://cooper.didichuxing.com/didocs/1",
		"finalSummary": "团队用于沉淀自动化操作流程。",
	}, http.StatusOK)
	if !updated.Data.Updated {
		t.Fatalf("team update mismatch: %+v", updated)
	}
	listed := doToolAPI[toolListEnvelope](t, h, http.MethodGet, "/api/tools/items?status=included&q=bob&sort=stars&take=1", nil, http.StatusOK)
	if listed.Data.Count != 1 || len(listed.Data.Items) != 1 || listed.Data.Items[0].CurrentSummarySource != "final" {
		t.Fatalf("team list mismatch: %+v", listed.Data)
	}

	for i := 0; i < 2; i++ {
		other := sampleToolRepo("node-team-extra-"+strconv.Itoa(i), "openai", "team-extra-"+strconv.Itoa(i))
		if _, err := store.ImportTeamTool(context.Background(), other, "seed", "", ""); err != nil {
			t.Fatalf("seed team tool %d: %v", i, err)
		}
	}
	latest := doToolAPI[toolListEnvelope](t, h, http.MethodGet, "/api/tools/items?status=included&sort=latest&take=1", nil, http.StatusOK)
	stars := doToolAPI[toolListEnvelope](t, h, http.MethodGet, "/api/tools/items?status=included&sort=stars&take=1", nil, http.StatusOK)
	if latest.Data.Count != 3 || stars.Data.Count != 3 || len(latest.Data.Items) != 1 || len(stars.Data.Items) != 1 {
		t.Fatalf("list count should describe all filtered team tools: latest=%+v stars=%+v", latest.Data, stars.Data)
	}

	missingReason := doToolAPI[toolAPIErrorData](t, h, http.MethodPost, "/api/tools/team/delete", map[string]any{
		"toolId":   imported.Data.Tool.ID,
		"operator": "carol",
	}, http.StatusBadRequest)
	if missingReason.Data.Class != tools.ErrorValidation {
		t.Fatalf("missing reason error mismatch: %+v", missingReason)
	}
	deleted := doToolAPI[map[string]bool](t, h, http.MethodPost, "/api/tools/team/delete", map[string]any{
		"toolId":   imported.Data.Tool.ID,
		"operator": "carol",
		"reason":   "团队不再使用",
	}, http.StatusOK)
	if !deleted.Data["deleted"] {
		t.Fatalf("team delete mismatch: %+v", deleted)
	}
	afterDelete := doToolAPI[toolListEnvelope](t, h, http.MethodGet, "/api/tools/items?status=included&q=team-tool", nil, http.StatusOK)
	if afterDelete.Data.Count != 0 {
		t.Fatalf("deleted team tool should leave included list: %+v", afterDelete.Data)
	}
}

func TestToolAPIListToolsPagination(t *testing.T) {
	store := liveToolAPIStore(t)
	h := newToolAPITestHandler(store, nil)

	high := sampleToolRepo("node-api-page-high", "openai", "api-page-high")
	high.Stars = 300
	mid := sampleToolRepo("node-api-page-mid", "openai", "api-page-mid")
	mid.Stars = 200
	low := sampleToolRepo("node-api-page-low", "openai", "api-page-low")
	low.Stars = 100
	for _, repo := range []tools.GitHubRepo{high, mid, low} {
		if _, err := store.ManualAdd(context.Background(), repo, "alice"); err != nil {
			t.Fatalf("ManualAdd %s: %v", repo.FullName, err)
		}
	}

	listed := doToolAPI[toolListEnvelope](t, h, http.MethodGet, "/api/tools/items?status=discovered&sort=stars&take=1&page=2", nil, http.StatusOK)
	if listed.Data.Count != 3 || listed.Data.Page != 2 || listed.Data.PageSize != 1 || listed.Data.Offset != 1 || listed.Data.Take != 1 {
		t.Fatalf("pagination envelope mismatch: %+v", listed.Data)
	}
	if len(listed.Data.Items) != 1 || listed.Data.Items[0].GitHubFullName != "openai/api-page-mid" {
		t.Fatalf("page 2 should return second item after star sorting: %+v", listed.Data.Items)
	}
}

func TestToolAPIBadRequestReturnsEnvelope(t *testing.T) {
	store := liveToolAPIStore(t)
	h := newToolAPITestHandler(store, nil)

	bad := doToolAPI[toolAPIErrorData](t, h, http.MethodPost, "/api/tools/configs", map[string]any{
		"name":        "高级查询",
		"method":      "keyword",
		"terms":       []string{"stars:>1000"},
		"triggerMode": "manual",
		"actor":       "alice",
	}, http.StatusBadRequest)
	if bad.Errno != 400001 || bad.Data.Class != tools.ErrorValidation {
		t.Fatalf("bad request envelope mismatch: %+v", bad)
	}
}

func TestToolAPINotFound(t *testing.T) {
	h := NewToolAPIHandler(nil, nil)
	env := doToolAPI[toolAPIErrorData](t, h, http.MethodGet, "/api/tools/nope", nil, http.StatusNotFound)
	if env.Errno != 404001 || env.Errmsg == "" {
		t.Fatalf("not found envelope mismatch: %+v", env)
	}
}
