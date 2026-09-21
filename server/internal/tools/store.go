package tools

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaFS embed.FS

// Store is the tools repository over a pgx pool.
type Store struct {
	pool *pgxpool.Pool
}

const defaultRuntimeSettingsID = "default"

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) EnsureSchema(ctx context.Context) error {
	sql, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, string(sql))
	return err
}

func DefaultRuntimeSettings() ToolRuntimeSettings {
	return ToolRuntimeSettings{
		ID:                         defaultRuntimeSettingsID,
		GitHubTokens:               []string{},
		GitHubTokenStrategy:        GitHubTokenStrategyRoundRobin,
		IncludeDefaultGitHubTokens: true,
		GitHubMaxPages:             DefaultGitHubMaxPages,
		GitHubPerPage:              DefaultGitHubPerPage,
		GitHubRequestIntervalMS:    int(DefaultGitHubRequestInterval / time.Millisecond),
		StarSnapshotLimit:          DefaultStarSnapshotLimit,
	}
}

func (s *Store) GetRuntimeSettings(ctx context.Context) (ToolRuntimeSettings, error) {
	settings, found, err := s.FindRuntimeSettings(ctx)
	if err != nil {
		return ToolRuntimeSettings{}, err
	}
	if !found {
		return DefaultRuntimeSettings(), nil
	}
	return settings, nil
}

func (s *Store) FindRuntimeSettings(ctx context.Context) (ToolRuntimeSettings, bool, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, github_tokens::text, github_base_url,
		       github_token_strategy, github_active_token_index, include_default_github_tokens,
		       github_max_pages, github_per_page, github_request_interval_ms, star_snapshot_limit,
		       created_by, updated_by, created_at, updated_at
		FROM tool_runtime_settings
		WHERE id=$1`, defaultRuntimeSettingsID)
	settings, err := scanRuntimeSettings(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ToolRuntimeSettings{}, false, nil
	}
	if err != nil {
		return ToolRuntimeSettings{}, false, err
	}
	return withRuntimeDefaults(settings), true, nil
}

func (s *Store) UpsertRuntimeSettings(ctx context.Context, settings ToolRuntimeSettings) error {
	settings = withRuntimeDefaults(settings)
	if settings.ID == "" {
		settings.ID = defaultRuntimeSettingsID
	}
	settings.GitHubTokens = NormalizeGitHubTokens(settings.GitHubTokens)
	if strings.TrimSpace(settings.CreatedBy) == "" {
		settings.CreatedBy = settings.UpdatedBy
	}
	if strings.TrimSpace(settings.CreatedBy) == "" {
		settings.CreatedBy = "system"
	}
	if strings.TrimSpace(settings.UpdatedBy) == "" {
		settings.UpdatedBy = settings.CreatedBy
	}
	tokens, err := json.Marshal(settings.GitHubTokens)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO tool_runtime_settings (
			id, github_tokens, github_base_url, github_token_strategy,
			github_active_token_index, include_default_github_tokens, github_max_pages,
			github_per_page, github_request_interval_ms, star_snapshot_limit,
			created_by, updated_by, updated_at
		) VALUES ($1,$2::jsonb,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now())
		ON CONFLICT (id) DO UPDATE SET
			github_tokens = EXCLUDED.github_tokens,
			github_base_url = EXCLUDED.github_base_url,
			github_token_strategy = EXCLUDED.github_token_strategy,
			github_active_token_index = EXCLUDED.github_active_token_index,
			include_default_github_tokens = EXCLUDED.include_default_github_tokens,
			github_max_pages = EXCLUDED.github_max_pages,
			github_per_page = EXCLUDED.github_per_page,
			github_request_interval_ms = EXCLUDED.github_request_interval_ms,
			star_snapshot_limit = EXCLUDED.star_snapshot_limit,
			updated_by = EXCLUDED.updated_by,
			updated_at = now()`,
		settings.ID, string(tokens), settings.GitHubBaseURL, settings.GitHubTokenStrategy,
		settings.GitHubActiveTokenIndex, settings.IncludeDefaultGitHubTokens, settings.GitHubMaxPages,
		settings.GitHubPerPage, settings.GitHubRequestIntervalMS, settings.StarSnapshotLimit,
		settings.CreatedBy, settings.UpdatedBy)
	return err
}

func (s *Store) UpsertConfig(ctx context.Context, c DiscoveryConfig) error {
	terms, err := json.Marshal(c.Terms)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO tool_discovery_configs (
			id, name, method, terms, trigger_mode, enabled,
			created_by, updated_by, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,now())
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			method = EXCLUDED.method,
			terms = EXCLUDED.terms,
			trigger_mode = EXCLUDED.trigger_mode,
			enabled = EXCLUDED.enabled,
			deleted_at = NULL,
			deleted_by = NULL,
			updated_by = EXCLUDED.updated_by,
			updated_at = now()`,
		c.ID, c.Name, c.Method, string(terms), c.TriggerMode, c.Enabled, c.CreatedBy, c.UpdatedBy)
	return err
}

func (s *Store) GetConfig(ctx context.Context, id string) (DiscoveryConfig, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, name, method, terms::text, trigger_mode, enabled,
		       deleted_at, deleted_by, last_success_at, last_run_at,
		       last_run_status, last_result_count, last_pages_scanned,
		       last_pause_requested, last_failure_reason,
		       created_by, updated_by, created_at, updated_at
		FROM tool_discovery_configs
		WHERE id=$1 AND deleted_at IS NULL`, id)
	return scanConfig(row)
}

func (s *Store) ListConfigs(ctx context.Context) ([]DiscoveryConfig, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, method, terms::text, trigger_mode, enabled,
		       deleted_at, deleted_by, last_success_at, last_run_at,
		       last_run_status, last_result_count, last_pages_scanned,
		       last_pause_requested, last_failure_reason,
		       created_by, updated_by, created_at, updated_at
		FROM tool_discovery_configs
		WHERE deleted_at IS NULL
		ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DiscoveryConfig
	for rows.Next() {
		cfg, err := scanConfig(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cfg)
	}
	return out, rows.Err()
}

func (s *Store) ListRunnableConfigs(ctx context.Context, trigger TriggerMode) ([]DiscoveryConfig, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, method, terms::text, trigger_mode, enabled,
		       deleted_at, deleted_by, last_success_at, last_run_at,
		       last_run_status, last_result_count, last_pages_scanned,
		       last_pause_requested, last_failure_reason,
		       created_by, updated_by, created_at, updated_at
		FROM tool_discovery_configs
		WHERE deleted_at IS NULL
		  AND enabled = true
		  AND trigger_mode = $1
		ORDER BY updated_at ASC, id ASC`, trigger)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DiscoveryConfig
	for rows.Next() {
		cfg, err := scanConfig(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cfg)
	}
	return out, rows.Err()
}

func (s *Store) SetConfigEnabled(ctx context.Context, id string, enabled bool, actor string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tool_discovery_configs
		SET enabled=$2, updated_by=$3, updated_at=now()
		WHERE id=$1 AND deleted_at IS NULL`,
		id, enabled, actor)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) SoftDeleteConfig(ctx context.Context, id, actor string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)

	var existingID string
	if err := tx.QueryRow(ctx, `
		SELECT id
		FROM tool_discovery_configs
		WHERE id=$1 AND deleted_at IS NULL
		FOR UPDATE`, id).Scan(&existingID); err != nil {
		return err
	}

	var running bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM tool_discovery_runs
			WHERE config_id=$1 AND status IN ($2,$3) AND finished_at IS NULL
		)`, id, RunRunning, RunPaused).Scan(&running); err != nil {
		return err
	}
	if running {
		return statusConflict("config has an active discovery run")
	}

	tag, err := tx.Exec(ctx, `
		UPDATE tool_discovery_configs
		SET enabled=false, deleted_at=now(), deleted_by=$2, updated_by=$2, updated_at=now()
		WHERE id=$1 AND deleted_at IS NULL`,
		id, actor)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return tx.Commit(ctx)
}

func (s *Store) CreateRun(ctx context.Context, r DiscoveryRun) error {
	if r.Status == "" {
		r.Status = RunRunning
	}
	if r.NextPage <= 0 {
		r.NextPage = 1
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)

	if r.ConfigID != nil && strings.TrimSpace(*r.ConfigID) != "" && r.Status == RunRunning {
		if _, err := tx.Exec(ctx, `
			UPDATE tool_discovery_runs
			SET finished_at=now(),
			    status=$2,
			    error_class=COALESCE(error_class, $3),
			    error_message=COALESCE(error_message, $4),
			    updated_at=now()
			WHERE config_id=$1
			  AND status=$5
			  AND finished_at IS NULL
			  AND started_at < now() - interval '1 hour'`,
			*r.ConfigID, RunFailed, "stale_running", "running discovery expired", RunRunning); err != nil {
			return err
		}

		var runningID string
		err := tx.QueryRow(ctx, `
			SELECT id
			FROM tool_discovery_runs
			WHERE config_id=$1 AND status IN ($2,$3) AND finished_at IS NULL
			ORDER BY started_at DESC, id DESC
			LIMIT 1`,
			*r.ConfigID, RunRunning, RunPaused).Scan(&runningID)
		if err == nil {
			return statusConflict("discovery is already active")
		}
		if err != pgx.ErrNoRows {
			return err
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO tool_discovery_runs (
			id, config_id, trigger_source, actor, status,
			result_count, new_count, updated_count, skipped_count,
			pages_scanned, incomplete_results, truncated,
			pause_requested, next_query_index, next_page,
			error_class, error_message, rate_limited, rate_limit_reset_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
		r.ID, r.ConfigID, r.TriggerSource, r.Actor, r.Status,
		r.ResultCount, r.NewCount, r.UpdatedCount, r.SkippedCount,
		r.PagesScanned, r.IncompleteResults, r.Truncated,
		r.PauseRequested, r.NextQueryIndex, r.NextPage,
		r.ErrorClass, r.ErrorMessage, r.RateLimited, r.RateLimitResetAt)
	if err != nil {
		if isUniqueViolation(err) {
			return statusConflict("discovery is already active")
		}
		return err
	}
	if r.ConfigID != nil && strings.TrimSpace(*r.ConfigID) != "" && r.Status == RunRunning {
		if _, err := tx.Exec(ctx, `
			UPDATE tool_discovery_configs
			SET last_run_at=now(),
			    last_run_status=$2,
			    last_result_count=0,
			    last_pages_scanned=0,
			    last_pause_requested=false,
			    last_failure_reason=NULL,
			    updated_at=now()
			WHERE id=$1 AND deleted_at IS NULL`,
			*r.ConfigID, RunRunning); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) FinishRun(ctx context.Context, r RunResult) error {
	finishedAt := r.FinishedAt
	if finishedAt == nil {
		now := time.Now().UTC()
		finishedAt = &now
	}
	errorClass := nullableString(r.ErrorClass)
	errorMessage := nullableString(r.ErrorMessage)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)

	var configID *string
	tag, err := tx.Exec(ctx, `
		UPDATE tool_discovery_runs
		SET finished_at=$2, status=$3, result_count=$4,
		    new_count=$5, updated_count=$6, skipped_count=$7,
		    pages_scanned=$8, incomplete_results=$9, truncated=$10,
		    pause_requested=false, next_query_index=$11, next_page=$12,
		    error_class=$13, error_message=$14, rate_limited=$15,
		    rate_limit_reset_at=$16, updated_at=now()
		WHERE id=$1`,
		r.ID, finishedAt, r.Status, r.ResultCount,
		r.NewCount, r.UpdatedCount, r.SkippedCount,
		r.PagesScanned, r.IncompleteResults, r.Truncated,
		nonNegative(r.NextQueryIndex), positiveOrDefault(r.NextPage, 1),
		errorClass, errorMessage, r.RateLimited, r.RateLimitResetAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	if err := tx.QueryRow(ctx, `SELECT config_id FROM tool_discovery_runs WHERE id=$1`, r.ID).Scan(&configID); err != nil {
		return err
	}
	if configID != nil {
		var successAt *time.Time
		if r.Status == RunSucceeded {
			successAt = finishedAt
		}
		_, err = tx.Exec(ctx, `
			UPDATE tool_discovery_configs
			SET last_run_at=$2,
			    last_success_at=COALESCE($3, last_success_at),
			    last_run_status=$4,
			    last_result_count=$5,
			    last_pages_scanned=$6,
			    last_pause_requested=false,
			    last_failure_reason=$7,
			    updated_at=now()
			WHERE id=$1`,
			*configID, finishedAt, successAt, r.Status, r.ResultCount, r.PagesScanned, errorMessage)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) UpdateRunProgress(ctx context.Context, r RunResult) error {
	if r.NextPage <= 0 {
		r.NextPage = 1
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)

	tag, err := tx.Exec(ctx, `
		UPDATE tool_discovery_runs
		SET result_count=$2,
		    new_count=$3,
		    updated_count=$4,
		    skipped_count=$5,
		    pages_scanned=$6,
		    incomplete_results=$7,
		    truncated=$8,
		    next_query_index=$9,
		    next_page=$10,
		    error_class=$11,
		    error_message=$12,
		    rate_limited=$13,
		    rate_limit_reset_at=$14,
		    updated_at=now()
		WHERE id=$1 AND status=$15 AND finished_at IS NULL`,
		r.ID, r.ResultCount, r.NewCount, r.UpdatedCount, r.SkippedCount,
		r.PagesScanned, r.IncompleteResults, r.Truncated,
		nonNegative(r.NextQueryIndex), positiveOrDefault(r.NextPage, 1),
		nullableString(r.ErrorClass), nullableString(r.ErrorMessage),
		r.RateLimited, r.RateLimitResetAt, RunRunning)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return statusConflict("discovery run is not running")
	}

	var configID *string
	if err := tx.QueryRow(ctx, `SELECT config_id FROM tool_discovery_runs WHERE id=$1`, r.ID).Scan(&configID); err != nil {
		return err
	}
	if configID != nil {
		_, err = tx.Exec(ctx, `
			UPDATE tool_discovery_configs
			SET last_run_at=now(),
			    last_run_status=$2,
			    last_result_count=$3,
			    last_pages_scanned=$4,
			    last_pause_requested=(
			        SELECT pause_requested
			        FROM tool_discovery_runs
			        WHERE id=$1
			    ),
			    last_failure_reason=$5,
			    updated_at=now()
			WHERE id=$6`,
			r.ID, RunRunning, r.ResultCount, r.PagesScanned, nullableString(r.ErrorMessage), *configID)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) RequestRunPause(ctx context.Context, configID, actor string) (DiscoveryRun, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return DiscoveryRun{}, err
	}
	defer rollback(ctx, tx)

	run, err := scanDiscoveryRun(tx.QueryRow(ctx, `
		SELECT `+runColumns("")+`
		FROM tool_discovery_runs
		WHERE config_id=$1 AND status IN ($2,$3) AND finished_at IS NULL
		ORDER BY started_at DESC, id DESC
		LIMIT 1
		FOR UPDATE`,
		configID, RunRunning, RunPaused))
	if err != nil {
		if err == pgx.ErrNoRows {
			return DiscoveryRun{}, statusConflict("discovery is not running")
		}
		return DiscoveryRun{}, err
	}
	if run.Status == RunPaused {
		return run, tx.Commit(ctx)
	}

	run, err = scanDiscoveryRun(tx.QueryRow(ctx, `
		UPDATE tool_discovery_runs
		SET pause_requested=true, updated_at=now()
		WHERE id=$1
		RETURNING `+runColumns(""),
		run.ID))
	if err != nil {
		return DiscoveryRun{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE tool_discovery_configs
		SET last_pause_requested=true,
		    updated_by=$2,
		    updated_at=now()
		WHERE id=$1 AND deleted_at IS NULL`,
		configID, actor); err != nil {
		return DiscoveryRun{}, err
	}
	return run, tx.Commit(ctx)
}

func (s *Store) PauseRun(ctx context.Context, r RunResult) error {
	if r.NextPage <= 0 {
		r.NextPage = 1
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)

	tag, err := tx.Exec(ctx, `
		UPDATE tool_discovery_runs
		SET status=$2,
		    result_count=$3,
		    new_count=$4,
		    updated_count=$5,
		    skipped_count=$6,
		    pages_scanned=$7,
		    incomplete_results=$8,
		    truncated=$9,
		    pause_requested=false,
		    next_query_index=$10,
		    next_page=$11,
		    error_class=$12,
		    error_message=$13,
		    rate_limited=$14,
		    rate_limit_reset_at=$15,
		    updated_at=now()
		WHERE id=$1 AND status=$16 AND finished_at IS NULL`,
		r.ID, RunPaused, r.ResultCount, r.NewCount, r.UpdatedCount, r.SkippedCount,
		r.PagesScanned, r.IncompleteResults, r.Truncated,
		nonNegative(r.NextQueryIndex), positiveOrDefault(r.NextPage, 1),
		nullableString(r.ErrorClass), nullableString(r.ErrorMessage),
		r.RateLimited, r.RateLimitResetAt, RunRunning)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return statusConflict("discovery run cannot be paused")
	}

	var configID *string
	if err := tx.QueryRow(ctx, `SELECT config_id FROM tool_discovery_runs WHERE id=$1`, r.ID).Scan(&configID); err != nil {
		return err
	}
	if configID != nil {
		_, err = tx.Exec(ctx, `
			UPDATE tool_discovery_configs
			SET last_run_at=now(),
			    last_run_status=$2,
			    last_result_count=$3,
			    last_pages_scanned=$4,
			    last_pause_requested=false,
			    last_failure_reason=NULL,
			    updated_at=now()
			WHERE id=$1`,
			*configID, RunPaused, r.ResultCount, r.PagesScanned)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) ResumeRun(ctx context.Context, configID, actor string) (DiscoveryRun, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return DiscoveryRun{}, err
	}
	defer rollback(ctx, tx)

	run, err := scanDiscoveryRun(tx.QueryRow(ctx, `
		UPDATE tool_discovery_runs
		SET status=$2,
		    pause_requested=false,
		    actor=$3,
		    updated_at=now()
		WHERE id = (
		    SELECT id
		    FROM tool_discovery_runs
		    WHERE config_id=$1 AND status=$4 AND finished_at IS NULL
		    ORDER BY started_at DESC, id DESC
		    LIMIT 1
		    FOR UPDATE
		)
		RETURNING `+runColumns(""),
		configID, RunRunning, actor, RunPaused))
	if err != nil {
		if err == pgx.ErrNoRows {
			return DiscoveryRun{}, statusConflict("discovery is not paused")
		}
		return DiscoveryRun{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE tool_discovery_configs
		SET last_run_status=$2,
		    last_pause_requested=false,
		    updated_by=$3,
		    updated_at=now()
		WHERE id=$1 AND deleted_at IS NULL`,
		configID, RunRunning, actor); err != nil {
		return DiscoveryRun{}, err
	}
	return run, tx.Commit(ctx)
}

func (s *Store) GetActiveRun(ctx context.Context, configID string) (DiscoveryRun, error) {
	return scanDiscoveryRun(s.pool.QueryRow(ctx, `
		SELECT `+runColumns("")+`
		FROM tool_discovery_runs
		WHERE config_id=$1 AND status IN ($2,$3) AND finished_at IS NULL
		ORDER BY started_at DESC, id DESC
		LIMIT 1`,
		configID, RunRunning, RunPaused))
}

func (s *Store) GetRun(ctx context.Context, runID string) (DiscoveryRun, error) {
	return scanDiscoveryRun(s.pool.QueryRow(ctx, `
		SELECT `+runColumns("")+`
		FROM tool_discovery_runs
		WHERE id=$1`,
		runID))
}

func (s *Store) IsRunPauseRequested(ctx context.Context, runID string) (bool, error) {
	var pauseRequested bool
	err := s.pool.QueryRow(ctx, `
		SELECT pause_requested
		FROM tool_discovery_runs
		WHERE id=$1 AND status=$2 AND finished_at IS NULL`,
		runID, RunRunning).Scan(&pauseRequested)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	return pauseRequested, err
}

func (s *Store) UpsertFromGitHub(ctx context.Context, repo GitHubRepo, source DiscoverySource) (UpsertResult, error) {
	if source.SourceType == "" {
		return UpsertResult{}, errors.New("source type is required")
	}
	if strings.TrimSpace(source.Term) == "" {
		return UpsertResult{}, errors.New("source term is required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return UpsertResult{}, err
	}
	defer rollback(ctx, tx)

	created, tool, err := upsertTool(ctx, tx, repo)
	if err != nil {
		return UpsertResult{}, err
	}
	if err := upsertSource(ctx, tx, tool.ID, source); err != nil {
		return UpsertResult{}, err
	}
	if isSnapshotEligibleStatus(tool.Status) {
		if err := saveStarSnapshot(ctx, tx, StarSnapshot{
			ID:         newID("snap"),
			ToolID:     tool.ID,
			Stars:      repo.Stars,
			Forks:      repo.Forks,
			OpenIssues: repo.OpenIssues,
			PushedAt:   repo.PushedAt,
		}); err != nil {
			return UpsertResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return UpsertResult{}, err
	}
	return UpsertResult{Tool: tool, Created: created}, nil
}

func (s *Store) ManualAdd(ctx context.Context, repo GitHubRepo, actor string, purposeTags ...[]string) (UpsertResult, error) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "manual"
	}
	if len(purposeTags) > 0 {
		repo.PurposeTags = NormalizePurposeTags(purposeTags[0])
		repo.PurposeTagsManuallySet = true
	}
	term := repo.FullName
	if term == "" && repo.Owner != "" && repo.Repo != "" {
		term = repo.Owner + "/" + repo.Repo
	}
	return s.UpsertFromGitHub(ctx, repo, DiscoverySource{
		SourceType: SourceManual,
		Term:       term,
		Actor:      actor,
	})
}

func (s *Store) ImportTeamTool(ctx context.Context, repo GitHubRepo, operator, finalSummary, cooperURL string, purposeTags ...[]string) (UpsertResult, error) {
	operator, err := requireText(operator, "operator", 80)
	if err != nil {
		return UpsertResult{}, err
	}
	summary, err := optionalText(finalSummary, "final_summary", 1000)
	if err != nil {
		return UpsertResult{}, err
	}
	cooper, err := validateOptionalCooperURL(cooperURL)
	if err != nil {
		return UpsertResult{}, err
	}
	if len(purposeTags) > 0 {
		repo.PurposeTags = NormalizePurposeTags(purposeTags[0])
		repo.PurposeTagsManuallySet = true
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return UpsertResult{}, err
	}
	defer rollback(ctx, tx)

	created, tool, err := upsertTool(ctx, tx, repo)
	if err != nil {
		return UpsertResult{}, err
	}
	if err := upsertSource(ctx, tx, tool.ID, DiscoverySource{
		SourceType: SourceManual,
		Term:       repo.FullName,
		Actor:      operator,
	}); err != nil {
		return UpsertResult{}, err
	}
	if err := saveStarSnapshot(ctx, tx, StarSnapshot{
		ID:         newID("snap"),
		ToolID:     tool.ID,
		Stars:      repo.Stars,
		Forks:      repo.Forks,
		OpenIssues: repo.OpenIssues,
		PushedAt:   repo.PushedAt,
	}); err != nil {
		return UpsertResult{}, err
	}

	updated, err := includeTeamTool(ctx, tx, tool.ID, summary, true)
	if err != nil {
		return UpsertResult{}, err
	}
	evalID := newID("eval")
	if err := insertCompletedIncludedEvaluation(ctx, tx, evalID, tool.ID, operator, cooper, summary); err != nil {
		return UpsertResult{}, err
	}
	if err := insertStatusEvent(ctx, tx, tool.ID, tool.Status, ToolIncluded, operator, "team_import", evalID); err != nil {
		return UpsertResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return UpsertResult{}, err
	}
	return UpsertResult{Tool: updated, Created: created}, nil
}

func (s *Store) UpdateTeamTool(ctx context.Context, toolID, operator, finalSummary, cooperURL string) error {
	operator, err := requireText(operator, "operator", 80)
	if err != nil {
		return err
	}
	summary, err := optionalText(finalSummary, "final_summary", 1000)
	if err != nil {
		return err
	}
	cooper, err := validateOptionalCooperURL(cooperURL)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)

	if _, err := includeTeamTool(ctx, tx, toolID, summary, false); err != nil {
		return err
	}
	evalID := newID("eval")
	if err := insertCompletedIncludedEvaluation(ctx, tx, evalID, toolID, operator, cooper, summary); err != nil {
		return err
	}
	if err := insertStatusEvent(ctx, tx, toolID, ToolIncluded, ToolIncluded, operator, "team_update", evalID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) DeleteTeamTool(ctx context.Context, toolID, reason, operator string) error {
	reason, err := requireText(reason, "reason", 500)
	if err != nil {
		return err
	}
	operator, err = requireText(operator, "operator", 80)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)

	tag, err := tx.Exec(ctx, `
		UPDATE tools
		SET status=$2,
		    excluded_at=now(),
		    excluded_stage=NULL,
		    excluded_reason=$3,
		    status_version=status_version+1,
		    updated_at=now()
		WHERE id=$1 AND status=$4`,
		toolID, ToolExcluded, reason, ToolIncluded)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return statusConflict("tool is not in team tools")
	}
	if err := insertStatusEvent(ctx, tx, toolID, ToolIncluded, ToolExcluded, operator, reason, ""); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) SaveStarSnapshot(ctx context.Context, snap StarSnapshot) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)

	if err := saveStarSnapshot(ctx, tx, snap); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func saveStarSnapshot(ctx context.Context, tx pgx.Tx, snap StarSnapshot) error {
	if snap.ID == "" {
		snap.ID = newID("snap")
	}
	capturedOn := snap.CapturedOn
	if capturedOn.IsZero() {
		if snap.SnapshotAt.IsZero() {
			capturedOn = time.Now().UTC()
		} else {
			capturedOn = snap.SnapshotAt
		}
	}
	snapshotAt := snap.SnapshotAt
	if snapshotAt.IsZero() {
		snapshotAt = capturedOn
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO tool_star_snapshots (
			id, tool_id, stars, forks, open_issues, pushed_at, captured_on, snapshot_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (tool_id, captured_on) DO UPDATE SET
			stars=EXCLUDED.stars,
			forks=EXCLUDED.forks,
			open_issues=EXCLUDED.open_issues,
			pushed_at=EXCLUDED.pushed_at,
			snapshot_at=EXCLUDED.snapshot_at`,
		snap.ID, snap.ToolID, snap.Stars, snap.Forks, snap.OpenIssues,
		snap.PushedAt, capturedOn.UTC(), snapshotAt.UTC()); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE tools t
		SET stars=$2,
		    forks=$3,
		    open_issues=$4,
		    pushed_at=COALESCE($5, t.pushed_at),
		    updated_at=now()
		WHERE t.id=$1
		  AND NOT EXISTS (
		      SELECT 1
		      FROM tool_star_snapshots newer
		      WHERE newer.tool_id=$1
		        AND newer.snapshot_at > $6
		  )`,
		snap.ToolID, snap.Stars, snap.Forks, snap.OpenIssues, snap.PushedAt, snapshotAt.UTC()); err != nil {
		return err
	}
	return nil
}

func isSnapshotEligibleStatus(status ToolStatus) bool {
	switch status {
	case ToolDiscovered, ToolEvaluating, ToolIncluded:
		return true
	default:
		return false
	}
}

func (s *Store) ListToolsMissingStarSnapshot(ctx context.Context, capturedOn time.Time, limit int) ([]Tool, error) {
	if capturedOn.IsZero() {
		capturedOn = time.Now().UTC()
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+toolColumns("t")+`, NULL::integer
		FROM tools t
		WHERE t.status IN ($1,$2,$3)
		  AND archived=false
		  AND fork=false
		  AND NOT EXISTS (
		      SELECT 1
		      FROM tool_star_snapshots s
		      WHERE s.tool_id=t.id AND s.captured_on=$4::date
		  )
		ORDER BY t.updated_at ASC, t.id ASC
		LIMIT $5`,
		ToolDiscovered, ToolEvaluating, ToolIncluded, capturedOn.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Tool
	for rows.Next() {
		var tool Tool
		var stars7D *int
		if err := scanTool(rows, &tool, &stars7D); err != nil {
			return nil, err
		}
		out = append(out, tool)
	}
	return out, rows.Err()
}

func (s *Store) ListTools(ctx context.Context, p ListToolsParams) ([]ToolListItem, error) {
	if p.Limit <= 0 {
		p.Limit = 50
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	if p.Now.IsZero() {
		p.Now = time.Now().UTC()
	}
	status := p.Status
	if status == "" {
		status = ToolDiscovered
	}

	order := "t.first_discovered_at DESC, t.stars DESC, t.id DESC"
	switch p.Sort {
	case SortStars:
		order = "t.stars DESC, t.first_discovered_at DESC, t.id DESC"
	case SortStars7D:
		order = "COALESCE(delta.stars_7d, -2147483648) DESC, t.stars DESC, t.id DESC"
	default:
		if status == ToolIncluded {
			order = "t.included_at DESC NULLS LAST, t.updated_at DESC, t.id DESC"
		}
	}

	rows, err := s.pool.Query(ctx, `
		SELECT `+toolColumns("t")+`, delta.stars_7d,
		       ev.id, ev.evaluator, ev.operator, ev.cooper_url, ev.started_at,
		       ev.completed_at, ev.result, ev.final_summary, ev.not_included_reason
		FROM tools t
		LEFT JOIN LATERAL (
			SELECT t.stars - baseline.stars AS stars_7d
			FROM (
				SELECT s.stars,
				       0 AS priority,
				       ABS(EXTRACT(EPOCH FROM (($3::timestamptz - interval '7 days') - s.snapshot_at))) AS sort_key
				FROM tool_star_snapshots s
				WHERE s.tool_id = t.id
				  AND s.snapshot_at >= $3::timestamptz - interval '8 days'
				  AND s.snapshot_at <= $3::timestamptz - interval '6 days'
				UNION ALL
				SELECT s.stars,
				       1 AS priority,
				       EXTRACT(EPOCH FROM s.snapshot_at) AS sort_key
				FROM tool_star_snapshots s
				WHERE s.tool_id = t.id
				  AND s.snapshot_at >= $3::timestamptz - interval '7 days'
				  AND s.snapshot_at < $3::timestamptz
				  AND EXISTS (
				      SELECT 1
				      FROM tool_star_snapshots newer
				      WHERE newer.tool_id = t.id
				        AND newer.snapshot_at > s.snapshot_at
				  )
			) baseline
			ORDER BY baseline.priority ASC, baseline.sort_key ASC
			LIMIT 1
		) delta ON true
		LEFT JOIN LATERAL (
			SELECT e.id, e.evaluator, e.operator, e.cooper_url, e.started_at,
			       e.completed_at, e.result, e.final_summary, e.not_included_reason
			FROM tool_evaluations e
			WHERE e.tool_id = t.id
			ORDER BY (e.completed_at IS NULL) DESC, e.started_at DESC, e.id DESC
			LIMIT 1
		) ev ON true
		WHERE t.status = $1
		  AND ($4::text IS NULL OR (
		       t.github_full_name ILIKE '%'||$4||'%'
		    OR t.description ILIKE '%'||$4||'%'
		    OR t.temporary_summary ILIKE '%'||$4||'%'
		    OR t.final_summary ILIKE '%'||$4||'%'
		    OR EXISTS (
		        SELECT 1
		        FROM jsonb_array_elements_text(t.purpose_tags) purpose(tag)
		        WHERE purpose.tag ILIKE '%'||$4||'%'
		    )
		    OR ev.evaluator ILIKE '%'||$4||'%'
		    OR ev.cooper_url ILIKE '%'||$4||'%'
		    OR EXISTS (
		        SELECT 1
		        FROM tool_discovery_sources s
		        WHERE s.tool_id=t.id
		          AND (s.term ILIKE '%'||$4||'%'
		           OR s.source_type ILIKE '%'||$4||'%')
		    )))
		  AND ($5::text[] IS NULL OR EXISTS (
		      SELECT 1
		      FROM tool_discovery_sources s
		      WHERE s.tool_id=t.id AND s.source_type = ANY($5::text[])
		  ))
		  AND ($7::text[] IS NULL OR EXISTS (
		      SELECT 1
		      FROM jsonb_array_elements_text(t.purpose_tags) purpose(tag)
		      WHERE purpose.tag = ANY($7::text[])
		  ))
		ORDER BY `+order+`
		LIMIT $2 OFFSET $6`,
		status, p.Limit, p.Now.UTC(), p.Q, listToolSourceTypesArg(p), p.Offset, listPurposeTagsArg(p))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ToolListItem
	for rows.Next() {
		item, err := scanToolListItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		sources, err := s.listSources(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Sources = sources
	}
	return out, nil
}

func (s *Store) GetTool(ctx context.Context, toolID string) (Tool, error) {
	var tool Tool
	var stars7D *int
	err := scanTool(s.pool.QueryRow(ctx, `
		SELECT `+toolColumns("t")+`, NULL::integer
		FROM tools t
		WHERE t.id = $1`,
		toolID), &tool, &stars7D)
	if err != nil {
		return Tool{}, err
	}
	return tool, nil
}

func (s *Store) UpdateTemporarySummary(ctx context.Context, toolID, summary, source string) error {
	summary, err := requireText(summary, "summary", 400)
	if err != nil {
		return err
	}
	source, err = requireText(source, "summarySource", 120)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE tools
		SET temporary_summary=$1,
		    temporary_summary_source=$2,
		    status_version=status_version+1,
		    updated_at=now()
		WHERE id=$3`,
		summary, source, toolID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) UpdateToolPurposeTags(ctx context.Context, toolID string, purposeTags []string, manuallySet bool) error {
	tags := NormalizePurposeTags(purposeTags)
	raw, err := json.Marshal(tags)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE tools
		SET purpose_tags=$2,
		    purpose_tags_manually_set=$3,
		    status_version=status_version+1,
		    updated_at=now()
		WHERE id=$1`,
		toolID, string(raw), manuallySet)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) ListToolsForPurposeClassification(ctx context.Context, limit int) ([]Tool, error) {
	args := []any{}
	limitClause := ""
	if limit > 0 {
		args = append(args, limit)
		limitClause = "LIMIT $1"
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+toolColumns("t")+`, NULL::integer
		FROM tools t
		WHERE t.purpose_tags_manually_set=false
		  AND t.status IN ('discovered', 'evaluating', 'included')
		ORDER BY t.updated_at DESC, t.id DESC
		`+limitClause, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Tool
	for rows.Next() {
		var tool Tool
		var stars7D *int
		if err := scanTool(rows, &tool, &stars7D); err != nil {
			return nil, err
		}
		out = append(out, tool)
	}
	return out, rows.Err()
}

func (s *Store) ToolStats(ctx context.Context, p ListToolsParams) (ToolListStats, error) {
	status := p.Status
	if status == "" {
		status = ToolDiscovered
	}
	stats := ToolListStats{Status: status}
	row := s.pool.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (
		          WHERE EXISTS (
		              SELECT 1 FROM tool_discovery_sources s
		              WHERE s.tool_id=t.id AND s.source_type=$4
		          )
		       ),
		       count(*) FILTER (
		          WHERE EXISTS (
		              SELECT 1 FROM tool_discovery_sources s
		              WHERE s.tool_id=t.id AND s.source_type=$5
		          )
		       ),
		       count(*) FILTER (
		          WHERE EXISTS (
		              SELECT 1 FROM tool_discovery_sources s
		              WHERE s.tool_id=t.id AND s.source_type=$6
		          )
		       ),
		       count(*) FILTER (WHERE ev.id IS NOT NULL AND ev.cooper_url IS NOT NULL),
		       count(*) FILTER (WHERE ev.id IS NOT NULL AND ev.cooper_url IS NULL),
		       count(DISTINCT ev.evaluator) FILTER (WHERE ev.evaluator IS NOT NULL AND ev.evaluator <> ''),
		       max(t.updated_at)
		FROM tools t
		LEFT JOIN LATERAL (
			SELECT e.id, e.evaluator, e.operator, e.cooper_url, e.started_at,
			       e.completed_at, e.result, e.final_summary, e.not_included_reason
			FROM tool_evaluations e
			WHERE e.tool_id = t.id
			ORDER BY (e.completed_at IS NULL) DESC, e.started_at DESC, e.id DESC
			LIMIT 1
		) ev ON true
		WHERE t.status = $1
		  AND ($2::text IS NULL OR (
		       t.github_full_name ILIKE '%'||$2||'%'
		    OR t.description ILIKE '%'||$2||'%'
		    OR t.temporary_summary ILIKE '%'||$2||'%'
		    OR t.final_summary ILIKE '%'||$2||'%'
		    OR EXISTS (
		        SELECT 1
		        FROM jsonb_array_elements_text(t.purpose_tags) purpose(tag)
		        WHERE purpose.tag ILIKE '%'||$2||'%'
		    )
		    OR ev.evaluator ILIKE '%'||$2||'%'
		    OR ev.cooper_url ILIKE '%'||$2||'%'
		    OR EXISTS (
		        SELECT 1
		        FROM tool_discovery_sources s
		        WHERE s.tool_id=t.id
		          AND (s.term ILIKE '%'||$2||'%'
		           OR s.source_type ILIKE '%'||$2||'%')
		    )))
		  AND ($3::text[] IS NULL OR EXISTS (
		      SELECT 1
		      FROM tool_discovery_sources s
		      WHERE s.tool_id=t.id AND s.source_type = ANY($3::text[])
		  ))
		  AND ($7::text[] IS NULL OR EXISTS (
		      SELECT 1
		      FROM jsonb_array_elements_text(t.purpose_tags) purpose(tag)
		      WHERE purpose.tag = ANY($7::text[])
		  ))`,
		status, p.Q, listToolSourceTypesArg(p), SourceKeyword, SourceTopic, SourceManual, listPurposeTagsArg(p))
	if err := row.Scan(
		&stats.Count,
		&stats.KeywordSourceCount,
		&stats.TopicSourceCount,
		&stats.ManualSourceCount,
		&stats.LinkedEvaluationCount,
		&stats.UnlinkedEvaluationCount,
		&stats.EvaluatorCount,
		&stats.LatestUpdatedAt,
	); err != nil {
		return ToolListStats{}, err
	}
	tags, err := s.ListPurposeTags(ctx, ListToolsParams{
		Status:      status,
		Q:           p.Q,
		SourceType:  p.SourceType,
		SourceTypes: p.SourceTypes,
	})
	if err != nil {
		return ToolListStats{}, err
	}
	stats.PurposeTags = tags
	return stats, nil
}

func listToolSourceTypesArg(p ListToolsParams) any {
	if len(p.SourceTypes) > 0 {
		out := make([]string, 0, len(p.SourceTypes))
		for _, source := range p.SourceTypes {
			out = append(out, string(source))
		}
		return out
	}
	if p.SourceType != nil {
		return []string{string(*p.SourceType)}
	}
	return nil
}

func listPurposeTagsArg(p ListToolsParams) any {
	tags := NormalizePurposeTags(p.PurposeTags)
	if len(tags) == 0 {
		return nil
	}
	return tags
}

func (s *Store) ListPurposeTags(ctx context.Context, p ListToolsParams) ([]string, error) {
	status := p.Status
	if status == "" {
		status = ToolDiscovered
	}
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT purpose.tag
		FROM tools t
		CROSS JOIN LATERAL jsonb_array_elements_text(t.purpose_tags) purpose(tag)
		LEFT JOIN LATERAL (
			SELECT e.id, e.evaluator, e.operator, e.cooper_url
			FROM tool_evaluations e
			WHERE e.tool_id = t.id
			ORDER BY (e.completed_at IS NULL) DESC, e.started_at DESC, e.id DESC
			LIMIT 1
		) ev ON true
		WHERE t.status = $1
		  AND purpose.tag <> ''
		  AND ($2::text IS NULL OR (
		       t.github_full_name ILIKE '%'||$2||'%'
		    OR t.description ILIKE '%'||$2||'%'
		    OR t.temporary_summary ILIKE '%'||$2||'%'
		    OR t.final_summary ILIKE '%'||$2||'%'
		    OR purpose.tag ILIKE '%'||$2||'%'
		    OR ev.evaluator ILIKE '%'||$2||'%'
		    OR ev.cooper_url ILIKE '%'||$2||'%'
		    OR EXISTS (
		        SELECT 1
		        FROM tool_discovery_sources s
		        WHERE s.tool_id=t.id
		          AND (s.term ILIKE '%'||$2||'%'
		           OR s.source_type ILIKE '%'||$2||'%')
		    )))
		  AND ($3::text[] IS NULL OR EXISTS (
		      SELECT 1
		      FROM tool_discovery_sources s
		      WHERE s.tool_id=t.id AND s.source_type = ANY($3::text[])
		  ))
		ORDER BY purpose.tag ASC`,
		status, p.Q, listToolSourceTypesArg(p))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		out = append(out, tag)
	}
	return out, rows.Err()
}

func (s *Store) StartEvaluation(ctx context.Context, toolID, evaluator, cooperURL, actor string) (Evaluation, error) {
	evaluator, err := requireText(evaluator, "evaluator", 80)
	if err != nil {
		return Evaluation{}, err
	}
	actor, err = requireText(actor, "operator", 80)
	if err != nil {
		return Evaluation{}, err
	}
	cooper, err := validateOptionalCooperURL(cooperURL)
	if err != nil {
		return Evaluation{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Evaluation{}, err
	}
	defer rollback(ctx, tx)

	tag, err := tx.Exec(ctx, `
		UPDATE tools
		SET status=$2, status_version=status_version+1, updated_at=now()
		WHERE id=$1 AND status=$3`,
		toolID, ToolEvaluating, ToolDiscovered)
	if err != nil {
		return Evaluation{}, err
	}
	if tag.RowsAffected() == 0 {
		return Evaluation{}, statusConflict("tool is not in discovered status")
	}

	evaluationID := newID("eval")
	eval, err := scanEvaluation(tx.QueryRow(ctx, `
		INSERT INTO tool_evaluations (
			id, tool_id, evaluator, operator, cooper_url
		) VALUES ($1,$2,$3,$4,$5)
		RETURNING `+evaluationColumns(""),
		evaluationID, toolID, evaluator, actor, cooper))
	if err != nil {
		return Evaluation{}, err
	}
	if err := insertStatusEvent(ctx, tx, toolID, ToolDiscovered, ToolEvaluating, actor, "start_evaluation", eval.ID); err != nil {
		return Evaluation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Evaluation{}, err
	}
	return eval, nil
}

func (s *Store) GetActiveEvaluation(ctx context.Context, toolID string) (Evaluation, error) {
	eval, err := scanEvaluation(s.pool.QueryRow(ctx, `
		SELECT `+evaluationColumns("")+`
		FROM tool_evaluations
		WHERE tool_id=$1 AND completed_at IS NULL`, toolID))
	if err == pgx.ErrNoRows {
		return Evaluation{}, statusConflict("active evaluation not found")
	}
	return eval, err
}

func (s *Store) UpdateEvaluationCooperURL(ctx context.Context, toolID, cooperURL, actor string) error {
	actor, err := requireText(actor, "operator", 80)
	if err != nil {
		return err
	}
	cooper, err := validateOptionalCooperURL(cooperURL)
	if err != nil {
		return err
	}
	if cooper == nil {
		return &DiscoveryError{Class: ErrorInvalidCooperURL, Message: "Cooper URL is required"}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)

	eval, err := scanEvaluation(tx.QueryRow(ctx, `
		UPDATE tool_evaluations e
		SET cooper_url=$2, updated_at=now()
		FROM tools t
		WHERE e.tool_id=t.id
		  AND e.tool_id=$1
		  AND e.completed_at IS NULL
		  AND t.status=$3
		RETURNING `+evaluationColumns("e"),
		toolID, cooper, ToolEvaluating))
	if err == pgx.ErrNoRows {
		return statusConflict("tool has no active evaluation")
	}
	if err != nil {
		return err
	}
	if err := insertStatusEvent(ctx, tx, toolID, ToolEvaluating, ToolEvaluating, actor, "update_cooper_url", eval.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ExcludeDiscovered(ctx context.Context, toolID, reason, actor string) error {
	reason, err := requireText(reason, "reason", 500)
	if err != nil {
		return err
	}
	actor, err = requireText(actor, "operator", 80)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)

	tag, err := tx.Exec(ctx, `
		UPDATE tools
		SET status=$2,
		    excluded_at=now(),
		    excluded_stage=$3,
		    excluded_reason=$4,
		    status_version=status_version+1,
		    updated_at=now()
		WHERE id=$1 AND status=$5`,
		toolID, ToolExcluded, ToolDiscovered, reason, ToolDiscovered)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return statusConflict("tool is not in discovered status")
	}
	if err := insertStatusEvent(ctx, tx, toolID, ToolDiscovered, ToolExcluded, actor, reason, ""); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) FinishEvaluation(ctx context.Context, req FinishEvaluationRequest) error {
	actor, err := requireText(req.Operator, "operator", 80)
	if err != nil {
		return err
	}
	var finalSummary string
	var notIncludedReason string
	switch req.Result {
	case EvaluationIncluded:
		finalSummary, err = requireTextRange(req.FinalSummary, "final_summary", 20, 1000)
	case EvaluationExcluded:
		notIncludedReason, err = requireText(req.NotIncludedReason, "not_included_reason", 1000)
	default:
		err = &DiscoveryError{Class: ErrorValidation, Message: "result must be included or excluded"}
	}
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)

	eval, err := scanEvaluation(tx.QueryRow(ctx, `
		SELECT `+evaluationColumns("")+`
		FROM tool_evaluations
		WHERE id=$1 AND tool_id=$2 AND completed_at IS NULL
		FOR UPDATE`, req.EvaluationID, req.ToolID))
	if err == pgx.ErrNoRows {
		return statusConflict("active evaluation not found")
	}
	if err != nil {
		return err
	}
	if eval.CooperURL == nil || strings.TrimSpace(*eval.CooperURL) == "" {
		return &DiscoveryError{Class: ErrorMissingCooperURL, Message: "Cooper URL is required before finishing evaluation"}
	}

	var rowsAffected int64
	switch req.Result {
	case EvaluationIncluded:
		tag, err := tx.Exec(ctx, `
			UPDATE tools
			SET status=$2,
			    final_summary=$3,
			    included_at=now(),
			    status_version=status_version+1,
			    updated_at=now()
			WHERE id=$1 AND status=$4`,
			req.ToolID, ToolIncluded, finalSummary, ToolEvaluating)
		if err != nil {
			return err
		}
		rowsAffected = tag.RowsAffected()
	case EvaluationExcluded:
		tag, err := tx.Exec(ctx, `
			UPDATE tools
			SET status=$2,
			    excluded_at=now(),
			    excluded_stage=$3,
			    excluded_reason=$4,
			    status_version=status_version+1,
			    updated_at=now()
			WHERE id=$1 AND status=$5`,
			req.ToolID, ToolExcluded, ToolEvaluating, notIncludedReason, ToolEvaluating)
		if err != nil {
			return err
		}
		rowsAffected = tag.RowsAffected()
	}
	if rowsAffected == 0 {
		return statusConflict("tool is not in evaluating status")
	}

	var summaryArg *string
	if finalSummary != "" {
		summaryArg = &finalSummary
	}
	var reasonArg *string
	if notIncludedReason != "" {
		reasonArg = &notIncludedReason
	}
	if _, err := tx.Exec(ctx, `
		UPDATE tool_evaluations
		SET completed_at=now(),
		    result=$2,
		    final_summary=$3,
		    not_included_reason=$4,
		    updated_at=now()
		WHERE id=$1 AND completed_at IS NULL`,
		req.EvaluationID, req.Result, summaryArg, reasonArg); err != nil {
		return err
	}
	note := string(req.Result)
	if req.Result == EvaluationExcluded {
		note = notIncludedReason
	}
	if err := insertStatusEvent(ctx, tx, req.ToolID, ToolEvaluating, ToolStatus(req.Result), actor, note, req.EvaluationID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func upsertTool(ctx context.Context, tx pgx.Tx, repo GitHubRepo) (bool, Tool, error) {
	id := newID("tool")
	homepageURL := nullableString(repo.HomepageURL)
	description := nullableString(repo.Description)
	summary := nullableString(repo.Summary)
	summarySource := nullableString(repo.TemporarySummarySource)
	license := nullableString(repo.LicenseSPDX)
	defaultBranch := nullableString(repo.DefaultBranch)
	topics, err := json.Marshal(repo.Topics)
	if err != nil {
		return false, Tool{}, err
	}
	purposeTags := NormalizePurposeTags(repo.PurposeTags)
	if len(purposeTags) == 0 && !repo.PurposeTagsManuallySet {
		purposeTags = InferPurposeTags(repo)
	}
	purposeTagsRaw, err := json.Marshal(purposeTags)
	if err != nil {
		return false, Tool{}, err
	}

	row := tx.QueryRow(ctx, `
		INSERT INTO tools (
			id, github_node_id, github_owner, github_repo, github_full_name,
			github_url, homepage_url, name, description, temporary_summary,
			temporary_summary_source, stars, forks, open_issues, topics,
			purpose_tags, purpose_tags_manually_set, license_spdx, default_branch,
			pushed_at, archived, fork, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)
		ON CONFLICT (github_node_id) DO UPDATE SET
			github_owner=EXCLUDED.github_owner,
			github_repo=EXCLUDED.github_repo,
			github_full_name=EXCLUDED.github_full_name,
			github_url=EXCLUDED.github_url,
			homepage_url=EXCLUDED.homepage_url,
			name=EXCLUDED.name,
			description=EXCLUDED.description,
			temporary_summary=COALESCE(EXCLUDED.temporary_summary, tools.temporary_summary),
			temporary_summary_source=COALESCE(EXCLUDED.temporary_summary_source, tools.temporary_summary_source),
			stars=EXCLUDED.stars,
			forks=EXCLUDED.forks,
			open_issues=EXCLUDED.open_issues,
			topics=EXCLUDED.topics,
			purpose_tags=CASE
				WHEN EXCLUDED.purpose_tags_manually_set THEN EXCLUDED.purpose_tags
				WHEN tools.purpose_tags_manually_set THEN tools.purpose_tags
				ELSE EXCLUDED.purpose_tags
			END,
			purpose_tags_manually_set=tools.purpose_tags_manually_set OR EXCLUDED.purpose_tags_manually_set,
			license_spdx=EXCLUDED.license_spdx,
			default_branch=EXCLUDED.default_branch,
			pushed_at=EXCLUDED.pushed_at,
			archived=EXCLUDED.archived,
			fork=EXCLUDED.fork,
			last_discovered_at=now(),
			updated_at=now()
		RETURNING `+toolColumns("")+`, (xmax = 0) AS created`,
		id, repo.NodeID, repo.Owner, repo.Repo, repo.FullName,
		repo.URL, homepageURL, repo.Name, description, summary,
		summarySource, repo.Stars, repo.Forks, repo.OpenIssues, string(topics),
		string(purposeTagsRaw), repo.PurposeTagsManuallySet, license, defaultBranch,
		repo.PushedAt, repo.Archived, repo.Fork, ToolDiscovered)

	var tool Tool
	var created bool
	if err := scanToolWithCreated(row, &tool, &created); err != nil {
		return false, Tool{}, err
	}
	return created, tool, nil
}

func upsertSource(ctx context.Context, tx pgx.Tx, toolID string, source DiscoverySource) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO tool_discovery_sources (
			tool_id, config_id, source_type, term, added_by
		) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (tool_id, (COALESCE(config_id, '')), source_type, term) DO UPDATE SET
			last_seen_at=now(),
			hit_count=tool_discovery_sources.hit_count + 1,
			added_by=EXCLUDED.added_by`,
		toolID, source.ConfigID, source.SourceType, source.Term, source.Actor)
	return err
}

func includeTeamTool(ctx context.Context, tx pgx.Tx, toolID string, summary *string, preserveSummary bool) (Tool, error) {
	summaryExpr := "$2"
	if preserveSummary {
		summaryExpr = "COALESCE($2, final_summary)"
	}
	row := tx.QueryRow(ctx, `
		UPDATE tools
		SET status=$3,
		    final_summary=`+summaryExpr+`,
		    included_at=CASE WHEN status=$3 THEN included_at ELSE now() END,
		    excluded_at=NULL,
		    excluded_stage=NULL,
		    excluded_reason=NULL,
		    status_version=CASE WHEN status=$3 THEN status_version ELSE status_version+1 END,
		    updated_at=now()
		WHERE id=$1
		  AND status IN ($3,$4,$5,$6)
		RETURNING `+toolColumns("")+`, NULL::integer`,
		toolID, summary, ToolIncluded, ToolDiscovered, ToolEvaluating, ToolExcluded)
	var tool Tool
	var stars7D *int
	if err := scanTool(row, &tool, &stars7D); err != nil {
		if err == pgx.ErrNoRows {
			return Tool{}, statusConflict("tool cannot be included as a team tool")
		}
		return Tool{}, err
	}
	return tool, nil
}

func insertCompletedIncludedEvaluation(ctx context.Context, tx pgx.Tx, id, toolID, operator string, cooperURL, finalSummary *string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO tool_evaluations (
			id, tool_id, evaluator, operator, cooper_url,
			completed_at, result, final_summary
		) VALUES ($1,$2,$3,$4,$5,now(),$6,$7)`,
		id, toolID, operator, operator, cooperURL, EvaluationIncluded, finalSummary)
	return err
}

func (s *Store) listSources(ctx context.Context, toolID string) ([]ToolSourceSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT source_type, term, config_id, last_seen_at, hit_count
		FROM tool_discovery_sources
		WHERE tool_id=$1
		ORDER BY last_seen_at DESC, source_type, term`, toolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ToolSourceSummary
	for rows.Next() {
		var item ToolSourceSummary
		if err := rows.Scan(&item.SourceType, &item.Term, &item.ConfigID, &item.LastSeenAt, &item.HitCount); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func insertStatusEvent(ctx context.Context, tx pgx.Tx, toolID string, from, to ToolStatus, actor, note, evaluationID string) error {
	var evalID *string
	if strings.TrimSpace(evaluationID) != "" {
		evalID = &evaluationID
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO tool_status_events (
			id, tool_id, from_status, to_status, actor, note, evaluation_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		newID("event"), toolID, from, to, actor, nullableString(note), evalID)
	return err
}

func scanRuntimeSettings(row pgx.Row) (ToolRuntimeSettings, error) {
	var settings ToolRuntimeSettings
	var tokensRaw string
	if err := row.Scan(
		&settings.ID, &tokensRaw, &settings.GitHubBaseURL, &settings.GitHubTokenStrategy,
		&settings.GitHubActiveTokenIndex, &settings.IncludeDefaultGitHubTokens, &settings.GitHubMaxPages,
		&settings.GitHubPerPage, &settings.GitHubRequestIntervalMS,
		&settings.StarSnapshotLimit, &settings.CreatedBy, &settings.UpdatedBy,
		&settings.CreatedAt, &settings.UpdatedAt,
	); err != nil {
		return settings, err
	}
	if err := json.Unmarshal([]byte(tokensRaw), &settings.GitHubTokens); err != nil {
		return settings, err
	}
	settings.GitHubTokens = NormalizeGitHubTokens(settings.GitHubTokens)
	return settings, nil
}

func withRuntimeDefaults(settings ToolRuntimeSettings) ToolRuntimeSettings {
	defaults := DefaultRuntimeSettings()
	if settings.ID == "" {
		settings.ID = defaults.ID
	}
	settings.GitHubTokens = NormalizeGitHubTokens(settings.GitHubTokens)
	settings.GitHubBaseURL = strings.TrimRight(strings.TrimSpace(settings.GitHubBaseURL), "/")
	if !IsValidGitHubTokenStrategy(settings.GitHubTokenStrategy) {
		settings.GitHubTokenStrategy = defaults.GitHubTokenStrategy
	}
	if settings.GitHubActiveTokenIndex < 0 {
		settings.GitHubActiveTokenIndex = defaults.GitHubActiveTokenIndex
	}
	if settings.GitHubMaxPages <= 0 {
		settings.GitHubMaxPages = defaults.GitHubMaxPages
	}
	if settings.GitHubPerPage <= 0 || settings.GitHubPerPage > DefaultGitHubPerPage {
		settings.GitHubPerPage = defaults.GitHubPerPage
	}
	if settings.GitHubRequestIntervalMS < 0 {
		settings.GitHubRequestIntervalMS = defaults.GitHubRequestIntervalMS
	}
	if settings.StarSnapshotLimit <= 0 {
		settings.StarSnapshotLimit = defaults.StarSnapshotLimit
	}
	return settings
}

func scanConfig(row pgx.Row) (DiscoveryConfig, error) {
	var cfg DiscoveryConfig
	var termsRaw string
	var lastRunStatus *string
	if err := row.Scan(
		&cfg.ID, &cfg.Name, &cfg.Method, &termsRaw, &cfg.TriggerMode,
		&cfg.Enabled, &cfg.DeletedAt, &cfg.DeletedBy, &cfg.LastSuccessAt,
		&cfg.LastRunAt, &lastRunStatus, &cfg.LastResultCount,
		&cfg.LastPagesScanned, &cfg.LastPauseRequested, &cfg.LastFailureReason,
		&cfg.CreatedBy, &cfg.UpdatedBy, &cfg.CreatedAt, &cfg.UpdatedAt,
	); err != nil {
		return cfg, err
	}
	if err := json.Unmarshal([]byte(termsRaw), &cfg.Terms); err != nil {
		return cfg, err
	}
	if lastRunStatus != nil {
		v := RunStatus(*lastRunStatus)
		cfg.LastRunStatus = &v
	}
	return cfg, nil
}

func scanDiscoveryRun(row pgx.Row) (DiscoveryRun, error) {
	var run DiscoveryRun
	if err := row.Scan(
		&run.ID, &run.ConfigID, &run.TriggerSource, &run.Actor,
		&run.StartedAt, &run.FinishedAt, &run.Status,
		&run.ResultCount, &run.NewCount, &run.UpdatedCount,
		&run.SkippedCount, &run.PagesScanned, &run.IncompleteResults,
		&run.Truncated, &run.PauseRequested, &run.NextQueryIndex,
		&run.NextPage, &run.ErrorClass, &run.ErrorMessage,
		&run.RateLimited, &run.RateLimitResetAt, &run.CreatedAt,
		&run.UpdatedAt,
	); err != nil {
		return run, err
	}
	return run, nil
}

func scanEvaluation(row pgx.Row) (Evaluation, error) {
	var eval Evaluation
	var result *string
	if err := row.Scan(
		&eval.ID, &eval.ToolID, &eval.Evaluator, &eval.Operator,
		&eval.CooperURL, &eval.StartedAt, &eval.CompletedAt,
		&result, &eval.FinalSummary, &eval.NotIncludedReason,
		&eval.CreatedAt, &eval.UpdatedAt,
	); err != nil {
		return eval, err
	}
	if result != nil {
		v := EvaluationResult(*result)
		eval.Result = &v
	}
	return eval, nil
}

func scanToolListItem(row pgx.Row) (ToolListItem, error) {
	var item ToolListItem
	var topicsRaw string
	var purposeTagsRaw string
	var evalID *string
	var evaluator *string
	var operator *string
	var cooperURL *string
	var startedAt *time.Time
	var completedAt *time.Time
	var result *string
	var finalSummary *string
	var notIncludedReason *string
	if err := row.Scan(
		&item.ID, &item.GitHubNodeID, &item.GitHubOwner, &item.GitHubRepo,
		&item.GitHubFullName, &item.GitHubURL, &item.HomepageURL,
		&item.Name, &item.Description, &item.TemporarySummary,
		&item.TemporarySummarySource, &item.FinalSummary, &item.Stars,
		&item.Forks, &item.OpenIssues, &topicsRaw, &purposeTagsRaw,
		&item.PurposeTagsManuallySet, &item.LicenseSPDX, &item.DefaultBranch,
		&item.PushedAt, &item.Archived, &item.Fork, &item.Status,
		&item.FirstDiscoveredAt, &item.LastDiscoveredAt, &item.IncludedAt,
		&item.ExcludedAt, &item.ExcludedStage, &item.ExcludedReason,
		&item.StatusVersion, &item.CreatedAt, &item.UpdatedAt, &item.Stars7D,
		&evalID, &evaluator, &operator, &cooperURL, &startedAt,
		&completedAt, &result, &finalSummary, &notIncludedReason,
	); err != nil {
		return item, err
	}
	if err := json.Unmarshal([]byte(topicsRaw), &item.Topics); err != nil {
		return item, err
	}
	if err := json.Unmarshal([]byte(purposeTagsRaw), &item.PurposeTags); err != nil {
		return item, err
	}
	if evalID != nil && evaluator != nil && operator != nil && startedAt != nil {
		item.Evaluation = &EvaluationSummary{
			ID:                *evalID,
			Evaluator:         *evaluator,
			Operator:          *operator,
			CooperURL:         cooperURL,
			StartedAt:         *startedAt,
			CompletedAt:       completedAt,
			FinalSummary:      finalSummary,
			NotIncludedReason: notIncludedReason,
		}
		if result != nil {
			v := EvaluationResult(*result)
			item.Evaluation.Result = &v
		}
	}
	return item, nil
}

func scanToolWithCreated(row pgx.Row, tool *Tool, created *bool) error {
	var topicsRaw string
	var purposeTagsRaw string
	err := row.Scan(
		&tool.ID, &tool.GitHubNodeID, &tool.GitHubOwner, &tool.GitHubRepo,
		&tool.GitHubFullName, &tool.GitHubURL, &tool.HomepageURL,
		&tool.Name, &tool.Description, &tool.TemporarySummary,
		&tool.TemporarySummarySource, &tool.FinalSummary, &tool.Stars,
		&tool.Forks, &tool.OpenIssues, &topicsRaw, &purposeTagsRaw,
		&tool.PurposeTagsManuallySet, &tool.LicenseSPDX, &tool.DefaultBranch,
		&tool.PushedAt, &tool.Archived, &tool.Fork, &tool.Status,
		&tool.FirstDiscoveredAt, &tool.LastDiscoveredAt, &tool.IncludedAt,
		&tool.ExcludedAt, &tool.ExcludedStage, &tool.ExcludedReason,
		&tool.StatusVersion, &tool.CreatedAt, &tool.UpdatedAt, created,
	)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(topicsRaw), &tool.Topics); err != nil {
		return err
	}
	return json.Unmarshal([]byte(purposeTagsRaw), &tool.PurposeTags)
}

func scanTool(row pgx.Row, tool *Tool, stars7D **int) error {
	var topicsRaw string
	var purposeTagsRaw string
	err := row.Scan(
		&tool.ID, &tool.GitHubNodeID, &tool.GitHubOwner, &tool.GitHubRepo,
		&tool.GitHubFullName, &tool.GitHubURL, &tool.HomepageURL,
		&tool.Name, &tool.Description, &tool.TemporarySummary,
		&tool.TemporarySummarySource, &tool.FinalSummary, &tool.Stars,
		&tool.Forks, &tool.OpenIssues, &topicsRaw, &purposeTagsRaw,
		&tool.PurposeTagsManuallySet, &tool.LicenseSPDX, &tool.DefaultBranch,
		&tool.PushedAt, &tool.Archived, &tool.Fork, &tool.Status,
		&tool.FirstDiscoveredAt, &tool.LastDiscoveredAt, &tool.IncludedAt,
		&tool.ExcludedAt, &tool.ExcludedStage, &tool.ExcludedReason,
		&tool.StatusVersion, &tool.CreatedAt, &tool.UpdatedAt, stars7D,
	)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(topicsRaw), &tool.Topics); err != nil {
		return err
	}
	return json.Unmarshal([]byte(purposeTagsRaw), &tool.PurposeTags)
}

func toolColumns(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return fmt.Sprintf(`%sid, %sgithub_node_id, %sgithub_owner, %sgithub_repo,
		%sgithub_full_name, %sgithub_url, %shomepage_url, %sname,
		%sdescription, %stemporary_summary, %stemporary_summary_source,
		%sfinal_summary, %sstars, %sforks, %sopen_issues, %stopics::text,
		%spurpose_tags::text, %spurpose_tags_manually_set, %slicense_spdx,
		%sdefault_branch, %spushed_at, %sarchived, %sfork, %sstatus,
		%sfirst_discovered_at, %slast_discovered_at, %sincluded_at,
		%sexcluded_at, %sexcluded_stage, %sexcluded_reason, %sstatus_version,
		%screated_at, %supdated_at`,
		prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix,
		prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix,
		prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix,
		prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix,
		prefix)
}

func runColumns(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return fmt.Sprintf(`%sid, %sconfig_id, %strigger_source, %sactor,
		%sstarted_at, %sfinished_at, %sstatus, %sresult_count, %snew_count,
		%supdated_count, %sskipped_count, %spages_scanned,
		%sincomplete_results, %struncated, %spause_requested,
		%snext_query_index, %snext_page, %serror_class, %serror_message,
		%srate_limited, %srate_limit_reset_at, %screated_at, %supdated_at`,
		prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix,
		prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix,
		prefix, prefix, prefix, prefix, prefix, prefix, prefix)
}

func evaluationColumns(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return fmt.Sprintf(`%sid, %stool_id, %sevaluator, %soperator,
		%scooper_url, %sstarted_at, %scompleted_at, %sresult,
		%sfinal_summary, %snot_included_reason, %screated_at, %supdated_at`,
		prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix,
		prefix, prefix, prefix, prefix)
}

func statusConflict(message string) error {
	return &DiscoveryError{Class: ErrorStatusConflict, Message: message}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func nullableString(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

func nonNegative(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func positiveOrDefault(v, fallback int) int {
	if v <= 0 {
		return fallback
	}
	return v
}

func optionalText(v, field string, maxLen int) (*string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	if len([]rune(v)) > maxLen {
		return nil, &DiscoveryError{Class: ErrorValidation, Message: field + " is too long"}
	}
	return &v, nil
}

func newID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}

func rollback(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(ctx)
}
