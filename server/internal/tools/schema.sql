CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS tool_discovery_configs (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    method              TEXT NOT NULL CHECK (method IN ('keyword', 'topic')),
    terms               JSONB NOT NULL,
    trigger_mode        TEXT NOT NULL CHECK (trigger_mode IN ('manual', 'weekly')),
    enabled             BOOLEAN NOT NULL DEFAULT TRUE,
    deleted_at          TIMESTAMPTZ,
    deleted_by          TEXT,
    last_success_at     TIMESTAMPTZ,
    last_run_at         TIMESTAMPTZ,
    last_run_status     TEXT CHECK (last_run_status IN ('running', 'paused', 'succeeded', 'partial', 'failed')),
    last_result_count   INTEGER NOT NULL DEFAULT 0,
    last_pages_scanned  INTEGER NOT NULL DEFAULT 0,
    last_pause_requested BOOLEAN NOT NULL DEFAULT FALSE,
    last_failure_reason TEXT,
    created_by          TEXT NOT NULL,
    updated_by          TEXT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (jsonb_typeof(terms) = 'array')
);

CREATE TABLE IF NOT EXISTS tool_discovery_runs (
    id                  TEXT PRIMARY KEY,
    config_id           TEXT REFERENCES tool_discovery_configs(id) ON DELETE SET NULL,
    trigger_source      TEXT NOT NULL CHECK (trigger_source IN ('manual', 'weekly')),
    actor               TEXT NOT NULL,
    started_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at         TIMESTAMPTZ,
    status              TEXT NOT NULL CHECK (status IN ('running', 'paused', 'succeeded', 'partial', 'failed')),
    result_count        INTEGER NOT NULL DEFAULT 0,
    new_count           INTEGER NOT NULL DEFAULT 0,
    updated_count       INTEGER NOT NULL DEFAULT 0,
    skipped_count       INTEGER NOT NULL DEFAULT 0,
    pages_scanned       INTEGER NOT NULL DEFAULT 0,
    incomplete_results  BOOLEAN NOT NULL DEFAULT FALSE,
    truncated           BOOLEAN NOT NULL DEFAULT FALSE,
    pause_requested     BOOLEAN NOT NULL DEFAULT FALSE,
    next_query_index    INTEGER NOT NULL DEFAULT 0,
    next_page           INTEGER NOT NULL DEFAULT 1,
    error_class         TEXT,
    error_message       TEXT,
    rate_limited        BOOLEAN NOT NULL DEFAULT FALSE,
    rate_limit_reset_at TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE tool_discovery_configs
    ADD COLUMN IF NOT EXISTS last_pages_scanned INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tool_discovery_configs
    ADD COLUMN IF NOT EXISTS last_pause_requested BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE tool_discovery_configs
    DROP CONSTRAINT IF EXISTS tool_discovery_configs_last_run_status_check;
ALTER TABLE tool_discovery_configs
    ADD CONSTRAINT tool_discovery_configs_last_run_status_check
    CHECK (last_run_status IN ('running', 'paused', 'succeeded', 'partial', 'failed'));

ALTER TABLE tool_discovery_runs
    ADD COLUMN IF NOT EXISTS pause_requested BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE tool_discovery_runs
    ADD COLUMN IF NOT EXISTS next_query_index INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tool_discovery_runs
    ADD COLUMN IF NOT EXISTS next_page INTEGER NOT NULL DEFAULT 1;
ALTER TABLE tool_discovery_runs
    DROP CONSTRAINT IF EXISTS tool_discovery_runs_status_check;
ALTER TABLE tool_discovery_runs
    ADD CONSTRAINT tool_discovery_runs_status_check
    CHECK (status IN ('running', 'paused', 'succeeded', 'partial', 'failed'));
ALTER TABLE tool_discovery_runs
    DROP CONSTRAINT IF EXISTS tool_discovery_runs_next_query_index_check;
ALTER TABLE tool_discovery_runs
    ADD CONSTRAINT tool_discovery_runs_next_query_index_check
    CHECK (next_query_index >= 0);
ALTER TABLE tool_discovery_runs
    DROP CONSTRAINT IF EXISTS tool_discovery_runs_next_page_check;
ALTER TABLE tool_discovery_runs
    ADD CONSTRAINT tool_discovery_runs_next_page_check
    CHECK (next_page >= 1);

CREATE TABLE IF NOT EXISTS tool_runtime_settings (
    id                            TEXT PRIMARY KEY,
    github_tokens                 JSONB NOT NULL DEFAULT '[]'::jsonb,
    github_base_url               TEXT NOT NULL DEFAULT '',
    github_token_strategy         TEXT NOT NULL DEFAULT 'round_robin',
    github_active_token_index     INTEGER NOT NULL DEFAULT 0,
    include_default_github_tokens BOOLEAN NOT NULL DEFAULT TRUE,
    github_max_pages              INTEGER NOT NULL DEFAULT 5,
    github_per_page               INTEGER NOT NULL DEFAULT 100,
    github_request_interval_ms    INTEGER NOT NULL DEFAULT 2000,
    star_snapshot_limit           INTEGER NOT NULL DEFAULT 200,
    created_by                    TEXT NOT NULL,
    updated_by                    TEXT NOT NULL,
    created_at                    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (jsonb_typeof(github_tokens) = 'array'),
    CHECK (github_token_strategy IN ('round_robin', 'fixed', 'failover')),
    CHECK (github_active_token_index >= 0),
    CHECK (github_max_pages > 0),
    CHECK (github_per_page > 0 AND github_per_page <= 100),
    CHECK (github_request_interval_ms >= 0),
    CHECK (star_snapshot_limit > 0)
);

ALTER TABLE tool_runtime_settings
    ADD COLUMN IF NOT EXISTS github_tokens JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE tool_runtime_settings
    ADD COLUMN IF NOT EXISTS github_base_url TEXT NOT NULL DEFAULT '';
ALTER TABLE tool_runtime_settings
    ADD COLUMN IF NOT EXISTS github_token_strategy TEXT NOT NULL DEFAULT 'round_robin';
ALTER TABLE tool_runtime_settings
    ADD COLUMN IF NOT EXISTS github_active_token_index INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tool_runtime_settings
    ADD COLUMN IF NOT EXISTS include_default_github_tokens BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE tool_runtime_settings
    ADD COLUMN IF NOT EXISTS github_max_pages INTEGER NOT NULL DEFAULT 5;
ALTER TABLE tool_runtime_settings
    ADD COLUMN IF NOT EXISTS github_per_page INTEGER NOT NULL DEFAULT 100;
ALTER TABLE tool_runtime_settings
    ADD COLUMN IF NOT EXISTS github_request_interval_ms INTEGER NOT NULL DEFAULT 2000;
ALTER TABLE tool_runtime_settings
    ADD COLUMN IF NOT EXISTS star_snapshot_limit INTEGER NOT NULL DEFAULT 200;
ALTER TABLE tool_runtime_settings
    DROP CONSTRAINT IF EXISTS tool_runtime_settings_github_tokens_check;
ALTER TABLE tool_runtime_settings
    ADD CONSTRAINT tool_runtime_settings_github_tokens_check
    CHECK (jsonb_typeof(github_tokens) = 'array');
ALTER TABLE tool_runtime_settings
    DROP CONSTRAINT IF EXISTS tool_runtime_settings_github_token_strategy_check;
ALTER TABLE tool_runtime_settings
    ADD CONSTRAINT tool_runtime_settings_github_token_strategy_check
    CHECK (github_token_strategy IN ('round_robin', 'fixed', 'failover'));
ALTER TABLE tool_runtime_settings
    DROP CONSTRAINT IF EXISTS tool_runtime_settings_github_active_token_index_check;
ALTER TABLE tool_runtime_settings
    ADD CONSTRAINT tool_runtime_settings_github_active_token_index_check
    CHECK (github_active_token_index >= 0);

CREATE TABLE IF NOT EXISTS tools (
    id                       TEXT PRIMARY KEY,
    github_node_id           TEXT NOT NULL UNIQUE,
    github_owner             TEXT NOT NULL,
    github_repo              TEXT NOT NULL,
    github_full_name         TEXT NOT NULL,
    github_url               TEXT NOT NULL,
    homepage_url             TEXT,
    name                     TEXT NOT NULL,
    description              TEXT,
    temporary_summary        TEXT,
    temporary_summary_source TEXT,
    final_summary            TEXT,
    stars                    INTEGER NOT NULL DEFAULT 0,
    forks                    INTEGER NOT NULL DEFAULT 0,
    open_issues              INTEGER NOT NULL DEFAULT 0,
    topics                   JSONB NOT NULL DEFAULT '[]'::jsonb,
    purpose_tags             JSONB NOT NULL DEFAULT '[]'::jsonb,
    purpose_tags_manually_set BOOLEAN NOT NULL DEFAULT FALSE,
    license_spdx             TEXT,
    default_branch           TEXT,
    pushed_at                TIMESTAMPTZ,
    archived                 BOOLEAN NOT NULL DEFAULT FALSE,
    fork                     BOOLEAN NOT NULL DEFAULT FALSE,
    status                   TEXT NOT NULL CHECK (status IN ('discovered', 'evaluating', 'included', 'excluded')),
    first_discovered_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_discovered_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    included_at              TIMESTAMPTZ,
    excluded_at              TIMESTAMPTZ,
    excluded_stage           TEXT CHECK (excluded_stage IN ('discovered', 'evaluating')),
    excluded_reason          TEXT,
    status_version           INTEGER NOT NULL DEFAULT 1,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (github_owner, github_repo),
    CHECK (jsonb_typeof(purpose_tags) = 'array')
);

ALTER TABLE tools
    ADD COLUMN IF NOT EXISTS purpose_tags JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE tools
    ADD COLUMN IF NOT EXISTS purpose_tags_manually_set BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE tools
    DROP CONSTRAINT IF EXISTS tools_purpose_tags_check;
ALTER TABLE tools
    ADD CONSTRAINT tools_purpose_tags_check
    CHECK (jsonb_typeof(purpose_tags) = 'array');

CREATE TABLE IF NOT EXISTS tool_discovery_sources (
    tool_id       TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    config_id     TEXT REFERENCES tool_discovery_configs(id) ON DELETE SET NULL,
    source_type   TEXT NOT NULL CHECK (source_type IN ('keyword', 'topic', 'manual')),
    term          TEXT NOT NULL,
    added_by      TEXT NOT NULL,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    hit_count     INTEGER NOT NULL DEFAULT 1
);

CREATE UNIQUE INDEX IF NOT EXISTS tool_discovery_sources_unique_idx
    ON tool_discovery_sources (tool_id, (COALESCE(config_id, '')), source_type, term);

CREATE TABLE IF NOT EXISTS tool_evaluations (
    id                  TEXT PRIMARY KEY,
    tool_id             TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    evaluator           TEXT NOT NULL,
    operator            TEXT NOT NULL,
    cooper_url          TEXT,
    started_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at        TIMESTAMPTZ,
    result              TEXT CHECK (result IN ('included', 'excluded')),
    final_summary       TEXT,
    not_included_reason TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS tool_evaluations_one_active_idx
    ON tool_evaluations (tool_id)
    WHERE completed_at IS NULL;

CREATE TABLE IF NOT EXISTS tool_status_events (
    id            TEXT PRIMARY KEY,
    tool_id       TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    from_status   TEXT CHECK (from_status IN ('discovered', 'evaluating', 'included', 'excluded')),
    to_status     TEXT NOT NULL CHECK (to_status IN ('discovered', 'evaluating', 'included', 'excluded')),
    actor         TEXT NOT NULL,
    note          TEXT,
    evaluation_id TEXT REFERENCES tool_evaluations(id) ON DELETE SET NULL,
    occurred_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tool_member_reviews (
    id         TEXT PRIMARY KEY,
    tool_id    TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    reviewer   TEXT NOT NULL,
    operator   TEXT NOT NULL,
    content    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tool_star_snapshots (
    id          TEXT PRIMARY KEY,
    tool_id     TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    stars       INTEGER NOT NULL,
    forks       INTEGER NOT NULL DEFAULT 0,
    open_issues INTEGER NOT NULL DEFAULT 0,
    pushed_at   TIMESTAMPTZ,
    captured_on DATE NOT NULL,
    snapshot_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tool_id, captured_on)
);

CREATE INDEX IF NOT EXISTS tool_discovery_configs_active_idx
    ON tool_discovery_configs (enabled, trigger_mode, updated_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS tool_discovery_runs_config_latest_idx
    ON tool_discovery_runs (config_id, started_at DESC);

WITH duplicate_active_tool_discovery_runs AS (
    SELECT id,
           row_number() OVER (PARTITION BY config_id ORDER BY started_at DESC, id DESC) AS rn
    FROM tool_discovery_runs
    WHERE config_id IS NOT NULL
      AND status IN ('running', 'paused')
      AND finished_at IS NULL
)
UPDATE tool_discovery_runs
SET status = 'failed',
    finished_at = now(),
    pause_requested = false,
    error_class = COALESCE(error_class, 'duplicate_active'),
    error_message = COALESCE(error_message, 'duplicate active discovery was closed during schema migration'),
    updated_at = now()
WHERE id IN (
    SELECT id
    FROM duplicate_active_tool_discovery_runs
    WHERE rn > 1
);

DROP INDEX IF EXISTS tool_discovery_runs_one_running_config_idx;
CREATE UNIQUE INDEX IF NOT EXISTS tool_discovery_runs_one_active_config_idx
    ON tool_discovery_runs (config_id)
    WHERE config_id IS NOT NULL AND status IN ('running', 'paused') AND finished_at IS NULL;

CREATE INDEX IF NOT EXISTS tools_status_latest_idx
    ON tools (status, first_discovered_at DESC, stars DESC, id DESC);

CREATE INDEX IF NOT EXISTS tools_status_stars_idx
    ON tools (status, stars DESC, first_discovered_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS tools_status_included_idx
    ON tools (status, included_at DESC, id DESC)
    WHERE status = 'included';

CREATE INDEX IF NOT EXISTS tools_name_trgm_idx
    ON tools USING gin (github_full_name gin_trgm_ops);

CREATE INDEX IF NOT EXISTS tools_purpose_tags_idx
    ON tools USING gin (purpose_tags);

CREATE INDEX IF NOT EXISTS tool_member_reviews_tool_latest_idx
    ON tool_member_reviews (tool_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS tool_star_snapshots_lookup_idx
    ON tool_star_snapshots (tool_id, snapshot_at DESC);
