package tools

import (
	"context"
	"os"
	"testing"
	"time"

	"aihot-server/internal/db"
)

func newTestStore(t *testing.T) *Store {
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

	s := New(pool)
	if err := s.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `TRUNCATE tool_runtime_settings`); err != nil {
		t.Fatalf("truncate runtime settings: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `
		TRUNCATE tool_status_events, tool_star_snapshots, tool_evaluations,
			tool_discovery_sources, tool_discovery_runs, tools,
			tool_discovery_configs CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return s
}

func ptrTime(t time.Time) *time.Time { return &t }

func sampleRepo(nodeID, owner, repo string) GitHubRepo {
	pushed := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	return GitHubRepo{
		NodeID:        nodeID,
		Owner:         owner,
		Repo:          repo,
		FullName:      owner + "/" + repo,
		URL:           "https://github.com/" + owner + "/" + repo,
		HomepageURL:   "https://example.com/" + repo,
		Name:          repo,
		Description:   "A coding agent toolkit",
		Summary:       "用于构建 coding agent 的工具包",
		Stars:         100,
		Forks:         10,
		OpenIssues:    3,
		Topics:        []string{"ai-agents", "coding-agent"},
		LicenseSPDX:   "MIT",
		DefaultBranch: "main",
		PushedAt:      &pushed,
		Archived:      false,
		Fork:          false,
	}
}
