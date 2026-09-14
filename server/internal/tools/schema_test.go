package tools

import (
	"strings"
	"testing"
)

func TestEmbeddedSchemaCapturesRequiredToolContracts(t *testing.T) {
	raw, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		t.Fatalf("ReadFile schema.sql: %v", err)
	}
	schema := string(raw)

	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS tool_discovery_configs",
		"CREATE TABLE IF NOT EXISTS tool_discovery_runs",
		"CREATE TABLE IF NOT EXISTS tools",
		"CREATE TABLE IF NOT EXISTS tool_discovery_sources",
		"CREATE TABLE IF NOT EXISTS tool_evaluations",
		"CREATE TABLE IF NOT EXISTS tool_status_events",
		"CREATE TABLE IF NOT EXISTS tool_star_snapshots",
		"CHECK (status IN ('discovered', 'evaluating', 'included', 'excluded'))",
		"CHECK (method IN ('keyword', 'topic'))",
		"CHECK (trigger_mode IN ('manual', 'weekly'))",
		"CHECK (status IN ('running', 'paused', 'succeeded', 'partial', 'failed'))",
		"github_node_id           TEXT NOT NULL UNIQUE",
		"status_version           INTEGER NOT NULL DEFAULT 1",
		"deleted_at          TIMESTAMPTZ",
		"pause_requested     BOOLEAN NOT NULL DEFAULT FALSE",
		"next_query_index    INTEGER NOT NULL DEFAULT 0",
		"next_page           INTEGER NOT NULL DEFAULT 1",
		"tool_evaluations_one_active_idx",
		"tool_discovery_sources_unique_idx",
		"tool_discovery_runs_one_active_config_idx",
	} {
		if !strings.Contains(schema, want) {
			t.Fatalf("schema missing required contract %q", want)
		}
	}
}
