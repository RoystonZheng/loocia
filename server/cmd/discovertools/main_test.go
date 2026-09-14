package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"aihot-server/internal/db"
	"aihot-server/internal/tools"
)

func TestRunWeeklyCommandExecutesConfiguredDiscovery(t *testing.T) {
	dsn := os.Getenv("AI_TOOL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AI_TOOL_TEST_DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()
	store := tools.New(pool)
	if err := store.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		TRUNCATE tool_status_events, tool_star_snapshots, tool_evaluations,
			tool_discovery_sources, tool_discovery_runs, tools,
			tool_discovery_configs CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if err := store.UpsertConfig(ctx, tools.DiscoveryConfig{
		ID:          "cfg-weekly-command",
		Name:        "Weekly command",
		Method:      tools.DiscoveryTopic,
		Terms:       []string{"ai-agents"},
		TriggerMode: tools.TriggerWeekly,
		Enabled:     true,
		CreatedBy:   "alice",
		UpdatedBy:   "alice",
	}); err != nil {
		t.Fatalf("UpsertConfig: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/repositories":
			if got := r.URL.Query().Get("q"); got != "topic:ai-agents fork:false archived:false" {
				t.Fatalf("query = %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"total_count":        1,
				"incomplete_results": false,
				"items": []map[string]any{{
					"node_id":           "node-command",
					"name":              "agents",
					"full_name":         "openai/agents",
					"html_url":          "https://github.com/openai/agents",
					"description":       "Agent toolkit",
					"stargazers_count":  125,
					"forks_count":       12,
					"open_issues_count": 3,
					"topics":            []string{"ai-agents"},
					"license":           map[string]any{"spdx_id": "MIT"},
					"default_branch":    "main",
					"pushed_at":         "2026-09-10T00:00:00Z",
					"archived":          false,
					"fork":              false,
					"owner":             map[string]any{"login": "openai"},
				}},
			})
		case "/repos/openai/agents":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"node_id":           "node-command",
				"name":              "agents",
				"full_name":         "openai/agents",
				"html_url":          "https://github.com/openai/agents",
				"description":       "Agent toolkit",
				"stargazers_count":  125,
				"forks_count":       12,
				"open_issues_count": 3,
				"topics":            []string{"ai-agents"},
				"license":           map[string]any{"spdx_id": "MIT"},
				"default_branch":    "main",
				"pushed_at":         "2026-09-10T00:00:00Z",
				"archived":          false,
				"fork":              false,
				"owner":             map[string]any{"login": "openai"},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	t.Setenv("AI_TOOL_DATABASE_URL", dsn)
	t.Setenv("AI_TOOL_GITHUB_BASE_URL", srv.URL)
	t.Setenv("AI_TOOL_GITHUB_MAX_PAGES", "1")

	if err := run("weekly", []string{"-actor", "cron-test"}); err != nil {
		t.Fatalf("run weekly: %v", err)
	}

	items, err := store.ListTools(ctx, tools.ListToolsParams{Status: tools.ToolDiscovered, Limit: 10})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(items) != 1 || items[0].GitHubNodeID != "node-command" || items[0].Stars != 125 {
		t.Fatalf("tool mismatch: %+v", items)
	}

	var snapshotCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM tool_star_snapshots s
		JOIN tools t ON t.id=s.tool_id
		WHERE t.github_node_id='node-command'`).Scan(&snapshotCount); err != nil {
		t.Fatalf("snapshot count: %v", err)
	}
	if snapshotCount != 1 {
		t.Fatalf("weekly command should persist a star snapshot, got %d", snapshotCount)
	}
}
