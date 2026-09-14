package publicapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"aihot-server/internal/tools"

	"github.com/jackc/pgx/v5"
)

const toolAPIPrefix = "/api/tools"

type ToolStore interface {
	ListConfigs(ctx context.Context) ([]tools.DiscoveryConfig, error)
	GetConfig(ctx context.Context, id string) (tools.DiscoveryConfig, error)
	UpsertConfig(ctx context.Context, cfg tools.DiscoveryConfig) error
	SetConfigEnabled(ctx context.Context, id string, enabled bool, actor string) error
	SoftDeleteConfig(ctx context.Context, id, actor string) error
	GetTool(ctx context.Context, toolID string) (tools.Tool, error)
	UpdateTemporarySummary(ctx context.Context, toolID, summary, source string) error
	ListTools(ctx context.Context, p tools.ListToolsParams) ([]tools.ToolListItem, error)
	ToolStats(ctx context.Context, p tools.ListToolsParams) (tools.ToolListStats, error)
	StartEvaluation(ctx context.Context, toolID, evaluator, cooperURL, actor string) (tools.Evaluation, error)
	GetActiveEvaluation(ctx context.Context, toolID string) (tools.Evaluation, error)
	UpdateEvaluationCooperURL(ctx context.Context, toolID, cooperURL, actor string) error
	ExcludeDiscovered(ctx context.Context, toolID, reason, actor string) error
	FinishEvaluation(ctx context.Context, req tools.FinishEvaluationRequest) error
	UpdateTeamTool(ctx context.Context, toolID, operator, finalSummary, cooperURL string) error
	DeleteTeamTool(ctx context.Context, toolID, reason, operator string) error
}

type ToolDiscoverer interface {
	StartConfigRun(ctx context.Context, configID string, trigger tools.TriggerMode, actor string) (tools.RunResult, error)
	RunExistingConfig(ctx context.Context, configID, runID string) (tools.RunResult, error)
	RequestConfigPause(ctx context.Context, configID, actor string) (tools.RunResult, error)
	ResumeConfigRun(ctx context.Context, configID, actor string) (tools.RunResult, error)
	PreviewManualAdd(ctx context.Context, repoURL string) (tools.GitHubRepo, error)
	ManualAdd(ctx context.Context, repoURL, actor string) (tools.ManualAddResult, error)
	ImportTeamTool(ctx context.Context, repoURL, operator, finalSummary, cooperURL string) (tools.ManualAddResult, error)
}

type ToolAPIHandler struct {
	store            ToolStore
	discoverer       ToolDiscoverer
	summaryGenerator ToolSummaryGenerator
}

func NewToolAPIHandler(store ToolStore, discoverer ToolDiscoverer) *ToolAPIHandler {
	return &ToolAPIHandler{store: store, discoverer: discoverer}
}

func (h *ToolAPIHandler) WithSummaryGenerator(generator ToolSummaryGenerator) *ToolAPIHandler {
	h.summaryGenerator = generator
	return h
}

func (h *ToolAPIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, toolAPIPrefix)
	path = strings.Trim(path, "/")

	switch {
	case path == "configs" && r.Method == http.MethodGet:
		h.handleListConfigs(w, r)
	case path == "configs" && r.Method == http.MethodPost:
		h.handleSaveConfig(w, r)
	case path == "configs/enable" && r.Method == http.MethodPost:
		h.handleSetConfigEnabled(w, r)
	case path == "configs/delete" && r.Method == http.MethodPost:
		h.handleDeleteConfig(w, r)
	case path == "configs/run" && r.Method == http.MethodPost:
		h.handleRunConfig(w, r)
	case path == "configs/pause" && r.Method == http.MethodPost:
		h.handlePauseConfig(w, r)
	case path == "configs/resume" && r.Method == http.MethodPost:
		h.handleResumeConfig(w, r)
	case path == "items" && r.Method == http.MethodGet:
		h.handleListTools(w, r)
	case path == "summaries/url-key" && r.Method == http.MethodPost:
		h.handleToolSummaryByURLKey(w, r)
	case path == "manual/preview" && r.Method == http.MethodPost:
		h.handleManualPreview(w, r)
	case path == "manual/add" && r.Method == http.MethodPost:
		h.handleManualAdd(w, r)
	case path == "team/import" && r.Method == http.MethodPost:
		h.handleTeamImport(w, r)
	case path == "team/update" && r.Method == http.MethodPost:
		h.handleTeamUpdate(w, r)
	case path == "team/delete" && r.Method == http.MethodPost:
		h.handleTeamDelete(w, r)
	case path == "discovered/exclude" && r.Method == http.MethodPost:
		h.handleExcludeDiscovered(w, r)
	case path == "evaluations/start" && r.Method == http.MethodPost:
		h.handleStartEvaluation(w, r)
	case path == "evaluations/cooper-url" && r.Method == http.MethodPost:
		h.handleUpdateCooperURL(w, r)
	case path == "evaluations/finish" && r.Method == http.MethodPost:
		h.handleFinishEvaluation(w, r)
	default:
		writeToolError(w, http.StatusNotFound, 404001, "api not found", "")
	}
}

type toolAPIEnvelope struct {
	Errno  int    `json:"errno"`
	Errmsg string `json:"errmsg"`
	Data   any    `json:"data,omitempty"`
}

type toolAPIErrorData struct {
	Class string `json:"class,omitempty"`
}

func writeToolOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toolAPIEnvelope{Errno: 0, Errmsg: "ok", Data: data})
}

func writeToolError(w http.ResponseWriter, statusCode int, errno int, msg, class string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	env := toolAPIEnvelope{Errno: errno, Errmsg: msg}
	if class != "" {
		env.Data = toolAPIErrorData{Class: class}
	}
	_ = json.NewEncoder(w).Encode(env)
}

func writeToolErr(w http.ResponseWriter, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		writeToolError(w, http.StatusNotFound, 404001, "not found", "")
		return
	}
	var derr *tools.DiscoveryError
	if tools.AsDiscoveryError(err, &derr) {
		switch derr.Class {
		case tools.ErrorStatusConflict:
			writeToolError(w, http.StatusConflict, 409001, derr.Message, derr.Class)
		case tools.ErrorRateLimited:
			writeToolError(w, http.StatusTooManyRequests, 429001, derr.Message, derr.Class)
		case tools.ErrorGitHub, tools.ErrorNetwork:
			writeToolError(w, http.StatusBadGateway, 502001, derr.Message, derr.Class)
		default:
			writeToolError(w, http.StatusBadRequest, 400001, derr.Message, derr.Class)
		}
		return
	}
	writeToolError(w, http.StatusInternalServerError, 500001, "internal error", "")
}

func decodeToolJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return &tools.DiscoveryError{Class: tools.ErrorValidation, Message: "invalid JSON body"}
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return &tools.DiscoveryError{Class: tools.ErrorValidation, Message: "request body must contain exactly one JSON object"}
	}
	return nil
}

func requireToolStore(store ToolStore) error {
	if store == nil {
		return fmt.Errorf("tool store is not configured")
	}
	return nil
}

func requireToolDiscoverer(discoverer ToolDiscoverer) error {
	if discoverer == nil {
		return fmt.Errorf("tool discoverer is not configured")
	}
	return nil
}

type configListEnvelope struct {
	Count         int          `json:"count"`
	EnabledCount  int          `json:"enabledCount"`
	DisabledCount int          `json:"disabledCount"`
	Items         []configJSON `json:"items"`
}

type configJSON struct {
	ID                 string           `json:"id"`
	Name               string           `json:"name"`
	Method             string           `json:"method"`
	Terms              []string         `json:"terms"`
	TriggerMode        string           `json:"triggerMode"`
	Enabled            bool             `json:"enabled"`
	LastSuccessAt      *time.Time       `json:"lastSuccessAt,omitempty"`
	LastRunAt          *time.Time       `json:"lastRunAt,omitempty"`
	LastRunStatus      *tools.RunStatus `json:"lastRunStatus,omitempty"`
	LastResultCount    int              `json:"lastResultCount"`
	LastPagesScanned   int              `json:"lastPagesScanned"`
	LastPauseRequested bool             `json:"lastPauseRequested"`
	LastFailureReason  *string          `json:"lastFailureReason,omitempty"`
	CreatedBy          string           `json:"createdBy"`
	UpdatedBy          string           `json:"updatedBy"`
	CreatedAt          time.Time        `json:"createdAt"`
	UpdatedAt          time.Time        `json:"updatedAt"`
}

func (h *ToolAPIHandler) handleListConfigs(w http.ResponseWriter, r *http.Request) {
	if err := requireToolStore(h.store); err != nil {
		writeToolErr(w, err)
		return
	}
	cfgs, err := h.store.ListConfigs(r.Context())
	if err != nil {
		writeToolErr(w, err)
		return
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	methods, err := parseOptionalDiscoveryMethods(r.URL.Query()["method"])
	if err != nil {
		writeToolErr(w, err)
		return
	}
	triggers, err := parseOptionalTriggerModes(r.URL.Query()["triggerMode"])
	if err != nil {
		writeToolErr(w, err)
		return
	}
	env := configListEnvelope{Items: []configJSON{}}
	for _, cfg := range cfgs {
		if !matchesConfigQuery(cfg, q) || !matchesConfigFilters(cfg, methods, triggers) {
			continue
		}
		if cfg.Enabled {
			env.EnabledCount++
		} else {
			env.DisabledCount++
		}
		env.Items = append(env.Items, toConfigJSON(cfg))
	}
	env.Count = len(env.Items)
	writeToolOK(w, env)
}

type saveConfigRequest struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Method      string   `json:"method"`
	Terms       []string `json:"terms"`
	TriggerMode string   `json:"triggerMode"`
	Enabled     *bool    `json:"enabled"`
	Actor       string   `json:"actor"`
}

func (h *ToolAPIHandler) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	if err := requireToolStore(h.store); err != nil {
		writeToolErr(w, err)
		return
	}
	var req saveConfigRequest
	if err := decodeToolJSON(r, &req); err != nil {
		writeToolErr(w, err)
		return
	}
	name, err := requireToolText(req.Name, "name", 120)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	method, err := parseDiscoveryMethod(req.Method)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	trigger, err := parseTriggerMode(req.TriggerMode)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	terms, err := normalizeConfigTerms(method, req.Terms)
	if err != nil {
		writeToolErr(w, err)
		return
	}

	createdBy := ""
	enabled := true
	id := strings.TrimSpace(req.ID)
	var existing *tools.DiscoveryConfig
	if id != "" {
		got, err := h.store.GetConfig(r.Context(), id)
		if err != nil {
			writeToolErr(w, err)
			return
		}
		existing = &got
		createdBy = got.CreatedBy
		enabled = got.Enabled
	} else {
		id = newToolAPIID("cfg")
	}
	actor, err := resolveConfigActor(req.Actor, existing)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	if createdBy == "" {
		createdBy = actor
	}
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	cfg := tools.DiscoveryConfig{
		ID:          id,
		Name:        name,
		Method:      method,
		Terms:       terms,
		TriggerMode: trigger,
		Enabled:     enabled,
		CreatedBy:   createdBy,
		UpdatedBy:   actor,
	}
	if err := h.store.UpsertConfig(r.Context(), cfg); err != nil {
		writeToolErr(w, err)
		return
	}
	saved, err := h.store.GetConfig(r.Context(), id)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	writeToolOK(w, map[string]any{"config": toConfigJSON(saved)})
}

type setConfigEnabledRequest struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	Actor   string `json:"actor"`
}

func (h *ToolAPIHandler) handleSetConfigEnabled(w http.ResponseWriter, r *http.Request) {
	if err := requireToolStore(h.store); err != nil {
		writeToolErr(w, err)
		return
	}
	var req setConfigEnabledRequest
	if err := decodeToolJSON(r, &req); err != nil {
		writeToolErr(w, err)
		return
	}
	id, err := requireToolText(req.ID, "id", 120)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	cfg, err := h.store.GetConfig(r.Context(), id)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	actor, err := resolveConfigActor(req.Actor, &cfg)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	if err := h.store.SetConfigEnabled(r.Context(), id, req.Enabled, actor); err != nil {
		writeToolErr(w, err)
		return
	}
	cfg, err = h.store.GetConfig(r.Context(), id)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	writeToolOK(w, map[string]any{"config": toConfigJSON(cfg)})
}

type idActorRequest struct {
	ID    string `json:"id"`
	Actor string `json:"actor"`
}

func (h *ToolAPIHandler) handleDeleteConfig(w http.ResponseWriter, r *http.Request) {
	if err := requireToolStore(h.store); err != nil {
		writeToolErr(w, err)
		return
	}
	var req idActorRequest
	if err := decodeToolJSON(r, &req); err != nil {
		writeToolErr(w, err)
		return
	}
	id, err := requireToolText(req.ID, "id", 120)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	cfg, err := h.store.GetConfig(r.Context(), id)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	actor, err := resolveConfigActor(req.Actor, &cfg)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	if err := h.store.SoftDeleteConfig(r.Context(), id, actor); err != nil {
		writeToolErr(w, err)
		return
	}
	writeToolOK(w, map[string]bool{"deleted": true})
}

func (h *ToolAPIHandler) handleRunConfig(w http.ResponseWriter, r *http.Request) {
	if err := requireToolDiscoverer(h.discoverer); err != nil {
		writeToolErr(w, err)
		return
	}
	if err := requireToolStore(h.store); err != nil {
		writeToolErr(w, err)
		return
	}
	var req idActorRequest
	if err := decodeToolJSON(r, &req); err != nil {
		writeToolErr(w, err)
		return
	}
	id, err := requireToolText(req.ID, "id", 120)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	cfg, err := h.store.GetConfig(r.Context(), id)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	actor, err := resolveConfigActor(req.Actor, &cfg)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	result, err := h.discoverer.StartConfigRun(r.Context(), id, tools.TriggerManual, actor)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	h.continueConfigRun(id, result.ID)
	writeToolOK(w, map[string]any{"run": toRunJSON(result)})
}

func (h *ToolAPIHandler) handlePauseConfig(w http.ResponseWriter, r *http.Request) {
	if err := requireToolDiscoverer(h.discoverer); err != nil {
		writeToolErr(w, err)
		return
	}
	if err := requireToolStore(h.store); err != nil {
		writeToolErr(w, err)
		return
	}
	var req idActorRequest
	if err := decodeToolJSON(r, &req); err != nil {
		writeToolErr(w, err)
		return
	}
	id, err := requireToolText(req.ID, "id", 120)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	cfg, err := h.store.GetConfig(r.Context(), id)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	actor, err := resolveConfigActor(req.Actor, &cfg)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	result, err := h.discoverer.RequestConfigPause(r.Context(), id, actor)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	writeToolOK(w, map[string]any{"run": toRunJSON(result)})
}

func (h *ToolAPIHandler) handleResumeConfig(w http.ResponseWriter, r *http.Request) {
	if err := requireToolDiscoverer(h.discoverer); err != nil {
		writeToolErr(w, err)
		return
	}
	if err := requireToolStore(h.store); err != nil {
		writeToolErr(w, err)
		return
	}
	var req idActorRequest
	if err := decodeToolJSON(r, &req); err != nil {
		writeToolErr(w, err)
		return
	}
	id, err := requireToolText(req.ID, "id", 120)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	cfg, err := h.store.GetConfig(r.Context(), id)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	actor, err := resolveConfigActor(req.Actor, &cfg)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	result, err := h.discoverer.ResumeConfigRun(r.Context(), id, actor)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	h.continueConfigRun(id, result.ID)
	writeToolOK(w, map[string]any{"run": toRunJSON(result)})
}

func (h *ToolAPIHandler) continueConfigRun(configID, runID string) {
	go func() {
		_, _ = h.discoverer.RunExistingConfig(context.Background(), configID, runID)
	}()
}

type toolListEnvelope struct {
	Count    int            `json:"count"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
	Offset   int            `json:"offset"`
	Take     int            `json:"take"`
	Stats    toolListStats  `json:"stats"`
	Items    []toolItemJSON `json:"items"`
}

type toolListStats struct {
	Status                  string     `json:"status"`
	KeywordSourceCount      int        `json:"keywordSourceCount"`
	TopicSourceCount        int        `json:"topicSourceCount"`
	ManualSourceCount       int        `json:"manualSourceCount"`
	LinkedEvaluationCount   int        `json:"linkedEvaluationCount"`
	UnlinkedEvaluationCount int        `json:"unlinkedEvaluationCount"`
	EvaluatorCount          int        `json:"evaluatorCount"`
	LatestUpdatedAt         *time.Time `json:"latestUpdatedAt,omitempty"`
}

type toolItemJSON struct {
	ID                     string                 `json:"id"`
	Name                   string                 `json:"name"`
	GitHubNodeID           string                 `json:"githubNodeId"`
	GitHubOwner            string                 `json:"githubOwner"`
	GitHubRepo             string                 `json:"githubRepo"`
	GitHubFullName         string                 `json:"githubFullName"`
	GitHubURL              string                 `json:"githubUrl"`
	HomepageURL            *string                `json:"homepageUrl,omitempty"`
	Description            *string                `json:"description,omitempty"`
	TemporarySummary       *string                `json:"temporarySummary,omitempty"`
	TemporarySummarySource *string                `json:"temporarySummarySource,omitempty"`
	FinalSummary           *string                `json:"finalSummary,omitempty"`
	CurrentSummary         *string                `json:"currentSummary,omitempty"`
	CurrentSummarySource   string                 `json:"currentSummarySource"`
	Stars                  int                    `json:"stars"`
	Forks                  int                    `json:"forks"`
	OpenIssues             int                    `json:"openIssues"`
	Stars7D                *int                   `json:"stars7d,omitempty"`
	Topics                 []string               `json:"topics"`
	LicenseSPDX            *string                `json:"licenseSpdx,omitempty"`
	DefaultBranch          *string                `json:"defaultBranch,omitempty"`
	PushedAt               *time.Time             `json:"pushedAt,omitempty"`
	Status                 string                 `json:"status"`
	FirstDiscoveredAt      time.Time              `json:"firstDiscoveredAt"`
	LastDiscoveredAt       time.Time              `json:"lastDiscoveredAt"`
	IncludedAt             *time.Time             `json:"includedAt,omitempty"`
	ExcludedAt             *time.Time             `json:"excludedAt,omitempty"`
	ExcludedStage          *string                `json:"excludedStage,omitempty"`
	ExcludedReason         *string                `json:"excludedReason,omitempty"`
	StatusVersion          int                    `json:"statusVersion"`
	SummaryKey             string                 `json:"summaryKey"`
	Sources                []toolSourceJSON       `json:"sources"`
	Evaluation             *evaluationSummaryJSON `json:"evaluation,omitempty"`
	CreatedAt              time.Time              `json:"createdAt"`
	UpdatedAt              time.Time              `json:"updatedAt"`
}

type toolSourceJSON struct {
	SourceType string    `json:"sourceType"`
	Term       string    `json:"term"`
	ConfigID   *string   `json:"configId,omitempty"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	HitCount   int       `json:"hitCount"`
}

type evaluationSummaryJSON struct {
	ID                string     `json:"id"`
	Evaluator         string     `json:"evaluator"`
	Operator          string     `json:"operator"`
	CooperURL         *string    `json:"cooperUrl,omitempty"`
	StartedAt         time.Time  `json:"startedAt"`
	CompletedAt       *time.Time `json:"completedAt,omitempty"`
	Result            *string    `json:"result,omitempty"`
	FinalSummary      *string    `json:"finalSummary,omitempty"`
	NotIncludedReason *string    `json:"notIncludedReason,omitempty"`
}

func (h *ToolAPIHandler) handleListTools(w http.ResponseWriter, r *http.Request) {
	if err := requireToolStore(h.store); err != nil {
		writeToolErr(w, err)
		return
	}
	status, err := parseToolStatus(r.URL.Query().Get("status"))
	if err != nil {
		writeToolErr(w, err)
		return
	}
	sortMode, err := parseToolSort(r.URL.Query().Get("sort"))
	if err != nil {
		writeToolErr(w, err)
		return
	}
	sourceTypes, err := parseOptionalSourceTypes(r.URL.Query()["source"])
	if err != nil {
		writeToolErr(w, err)
		return
	}
	limit, err := parseToolLimit(r.URL.Query().Get("take"), 50, 200)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	offset, page, err := parseToolOffset(r.URL.Query().Get("offset"), r.URL.Query().Get("page"), limit)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	q := nullableToolQuery(r.URL.Query().Get("q"))
	params := tools.ListToolsParams{
		Status:      status,
		Sort:        sortMode,
		Q:           q,
		SourceTypes: sourceTypes,
		Limit:       limit,
		Offset:      offset,
		Now:         time.Now().UTC(),
	}
	stats, err := h.store.ToolStats(r.Context(), params)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	items, err := h.store.ListTools(r.Context(), params)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	env := toolListEnvelope{
		Count:    stats.Count,
		Page:     page,
		PageSize: limit,
		Offset:   offset,
		Take:     limit,
		Stats:    toToolListStatsJSON(stats),
		Items:    make([]toolItemJSON, 0, len(items)),
	}
	for _, item := range items {
		j := toToolItemJSON(item)
		env.Items = append(env.Items, j)
	}
	writeToolOK(w, env)
}

type manualRepoRequest struct {
	RepoURL string `json:"repoUrl"`
	Actor   string `json:"actor"`
}

func (h *ToolAPIHandler) handleManualPreview(w http.ResponseWriter, r *http.Request) {
	if err := requireToolDiscoverer(h.discoverer); err != nil {
		writeToolErr(w, err)
		return
	}
	var req manualRepoRequest
	if err := decodeToolJSON(r, &req); err != nil {
		writeToolErr(w, err)
		return
	}
	repo, err := h.discoverer.PreviewManualAdd(r.Context(), req.RepoURL)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	writeToolOK(w, map[string]any{"repository": toGitHubRepoJSON(repo)})
}

func (h *ToolAPIHandler) handleManualAdd(w http.ResponseWriter, r *http.Request) {
	if err := requireToolDiscoverer(h.discoverer); err != nil {
		writeToolErr(w, err)
		return
	}
	var req manualRepoRequest
	if err := decodeToolJSON(r, &req); err != nil {
		writeToolErr(w, err)
		return
	}
	actor, err := requireToolText(req.Actor, "actor", 80)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	result, err := h.discoverer.ManualAdd(r.Context(), req.RepoURL, actor)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	writeToolOK(w, map[string]any{
		"tool":      toToolJSON(result.Tool, nil, nil),
		"created":   result.Created,
		"duplicate": result.Duplicate,
	})
}

type teamImportRequest struct {
	RepoURL      string `json:"repoUrl"`
	Operator     string `json:"operator"`
	CooperURL    string `json:"cooperUrl"`
	FinalSummary string `json:"finalSummary"`
}

func (h *ToolAPIHandler) handleTeamImport(w http.ResponseWriter, r *http.Request) {
	if err := requireToolDiscoverer(h.discoverer); err != nil {
		writeToolErr(w, err)
		return
	}
	var req teamImportRequest
	if err := decodeToolJSON(r, &req); err != nil {
		writeToolErr(w, err)
		return
	}
	operator, err := requireToolText(req.Operator, "operator", 80)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	result, err := h.discoverer.ImportTeamTool(r.Context(), req.RepoURL, operator, req.FinalSummary, req.CooperURL)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	writeToolOK(w, map[string]any{
		"tool":      toToolJSON(result.Tool, nil, nil),
		"created":   result.Created,
		"duplicate": result.Duplicate,
	})
}

type teamUpdateRequest struct {
	ToolID       string `json:"toolId"`
	Operator     string `json:"operator"`
	CooperURL    string `json:"cooperUrl"`
	FinalSummary string `json:"finalSummary"`
}

func (h *ToolAPIHandler) handleTeamUpdate(w http.ResponseWriter, r *http.Request) {
	if err := requireToolStore(h.store); err != nil {
		writeToolErr(w, err)
		return
	}
	var req teamUpdateRequest
	if err := decodeToolJSON(r, &req); err != nil {
		writeToolErr(w, err)
		return
	}
	toolID, err := requireToolText(req.ToolID, "toolId", 120)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	operator, err := requireToolText(req.Operator, "operator", 80)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	if err := h.store.UpdateTeamTool(r.Context(), toolID, operator, req.FinalSummary, req.CooperURL); err != nil {
		writeToolErr(w, err)
		return
	}
	writeToolOK(w, map[string]bool{"updated": true})
}

type teamDeleteRequest struct {
	ToolID   string `json:"toolId"`
	Operator string `json:"operator"`
	Reason   string `json:"reason"`
}

func (h *ToolAPIHandler) handleTeamDelete(w http.ResponseWriter, r *http.Request) {
	if err := requireToolStore(h.store); err != nil {
		writeToolErr(w, err)
		return
	}
	var req teamDeleteRequest
	if err := decodeToolJSON(r, &req); err != nil {
		writeToolErr(w, err)
		return
	}
	toolID, err := requireToolText(req.ToolID, "toolId", 120)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	operator, err := requireToolText(req.Operator, "operator", 80)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	if err := h.store.DeleteTeamTool(r.Context(), toolID, req.Reason, operator); err != nil {
		writeToolErr(w, err)
		return
	}
	writeToolOK(w, map[string]bool{"deleted": true})
}

type excludeDiscoveredRequest struct {
	ToolID   string `json:"toolId"`
	Reason   string `json:"reason"`
	Operator string `json:"operator"`
}

func (h *ToolAPIHandler) handleExcludeDiscovered(w http.ResponseWriter, r *http.Request) {
	if err := requireToolStore(h.store); err != nil {
		writeToolErr(w, err)
		return
	}
	var req excludeDiscoveredRequest
	if err := decodeToolJSON(r, &req); err != nil {
		writeToolErr(w, err)
		return
	}
	toolID, err := requireToolText(req.ToolID, "toolId", 120)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	operator, err := requireToolText(req.Operator, "operator", 80)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	if err := h.store.ExcludeDiscovered(r.Context(), toolID, req.Reason, operator); err != nil {
		writeToolErr(w, err)
		return
	}
	writeToolOK(w, map[string]bool{"excluded": true})
}

type startEvaluationRequest struct {
	ToolID    string `json:"toolId"`
	Evaluator string `json:"evaluator"`
	Operator  string `json:"operator"`
	CooperURL string `json:"cooperUrl"`
}

func (h *ToolAPIHandler) handleStartEvaluation(w http.ResponseWriter, r *http.Request) {
	if err := requireToolStore(h.store); err != nil {
		writeToolErr(w, err)
		return
	}
	var req startEvaluationRequest
	if err := decodeToolJSON(r, &req); err != nil {
		writeToolErr(w, err)
		return
	}
	toolID, err := requireToolText(req.ToolID, "toolId", 120)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	eval, err := h.store.StartEvaluation(r.Context(), toolID, req.Evaluator, req.CooperURL, req.Operator)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	writeToolOK(w, map[string]any{"evaluation": toEvaluationJSON(eval)})
}

type updateCooperURLRequest struct {
	ToolID    string `json:"toolId"`
	CooperURL string `json:"cooperUrl"`
	Operator  string `json:"operator"`
}

func (h *ToolAPIHandler) handleUpdateCooperURL(w http.ResponseWriter, r *http.Request) {
	if err := requireToolStore(h.store); err != nil {
		writeToolErr(w, err)
		return
	}
	var req updateCooperURLRequest
	if err := decodeToolJSON(r, &req); err != nil {
		writeToolErr(w, err)
		return
	}
	toolID, err := requireToolText(req.ToolID, "toolId", 120)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	if err := h.store.UpdateEvaluationCooperURL(r.Context(), toolID, req.CooperURL, req.Operator); err != nil {
		writeToolErr(w, err)
		return
	}
	eval, err := h.store.GetActiveEvaluation(r.Context(), toolID)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	writeToolOK(w, map[string]any{"evaluation": toEvaluationJSON(eval)})
}

type finishEvaluationRequest struct {
	ToolID            string `json:"toolId"`
	EvaluationID      string `json:"evaluationId"`
	Result            string `json:"result"`
	Operator          string `json:"operator"`
	FinalSummary      string `json:"finalSummary"`
	NotIncludedReason string `json:"notIncludedReason"`
}

func (h *ToolAPIHandler) handleFinishEvaluation(w http.ResponseWriter, r *http.Request) {
	if err := requireToolStore(h.store); err != nil {
		writeToolErr(w, err)
		return
	}
	var req finishEvaluationRequest
	if err := decodeToolJSON(r, &req); err != nil {
		writeToolErr(w, err)
		return
	}
	toolID, err := requireToolText(req.ToolID, "toolId", 120)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	evalID, err := requireToolText(req.EvaluationID, "evaluationId", 120)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	result, err := parseEvaluationResult(req.Result)
	if err != nil {
		writeToolErr(w, err)
		return
	}
	if err := h.store.FinishEvaluation(r.Context(), tools.FinishEvaluationRequest{
		ToolID:            toolID,
		EvaluationID:      evalID,
		Result:            result,
		Operator:          req.Operator,
		FinalSummary:      req.FinalSummary,
		NotIncludedReason: req.NotIncludedReason,
	}); err != nil {
		writeToolErr(w, err)
		return
	}
	writeToolOK(w, map[string]bool{"completed": true})
}

func matchesConfigQuery(cfg tools.DiscoveryConfig, q string) bool {
	if q == "" {
		return true
	}
	values := []string{cfg.ID, cfg.Name, string(cfg.Method), string(cfg.TriggerMode)}
	values = append(values, cfg.Terms...)
	for _, v := range values {
		if strings.Contains(strings.ToLower(v), q) {
			return true
		}
	}
	return false
}

func toConfigJSON(cfg tools.DiscoveryConfig) configJSON {
	terms := cfg.Terms
	if terms == nil {
		terms = []string{}
	}
	return configJSON{
		ID:                 cfg.ID,
		Name:               cfg.Name,
		Method:             string(cfg.Method),
		Terms:              terms,
		TriggerMode:        string(cfg.TriggerMode),
		Enabled:            cfg.Enabled,
		LastSuccessAt:      cfg.LastSuccessAt,
		LastRunAt:          cfg.LastRunAt,
		LastRunStatus:      cfg.LastRunStatus,
		LastResultCount:    cfg.LastResultCount,
		LastPagesScanned:   cfg.LastPagesScanned,
		LastPauseRequested: cfg.LastPauseRequested,
		LastFailureReason:  cfg.LastFailureReason,
		CreatedBy:          cfg.CreatedBy,
		UpdatedBy:          cfg.UpdatedBy,
		CreatedAt:          cfg.CreatedAt,
		UpdatedAt:          cfg.UpdatedAt,
	}
}

func toToolListStatsJSON(stats tools.ToolListStats) toolListStats {
	return toolListStats{
		Status:                  string(stats.Status),
		KeywordSourceCount:      stats.KeywordSourceCount,
		TopicSourceCount:        stats.TopicSourceCount,
		ManualSourceCount:       stats.ManualSourceCount,
		LinkedEvaluationCount:   stats.LinkedEvaluationCount,
		UnlinkedEvaluationCount: stats.UnlinkedEvaluationCount,
		EvaluatorCount:          stats.EvaluatorCount,
		LatestUpdatedAt:         stats.LatestUpdatedAt,
	}
}

type runJSON struct {
	ID                string     `json:"id"`
	Status            string     `json:"status"`
	FinishedAt        *time.Time `json:"finishedAt,omitempty"`
	ResultCount       int        `json:"resultCount"`
	NewCount          int        `json:"newCount"`
	UpdatedCount      int        `json:"updatedCount"`
	SkippedCount      int        `json:"skippedCount"`
	PagesScanned      int        `json:"pagesScanned"`
	IncompleteResults bool       `json:"incompleteResults"`
	Truncated         bool       `json:"truncated"`
	PauseRequested    bool       `json:"pauseRequested"`
	NextQueryIndex    int        `json:"nextQueryIndex"`
	NextPage          int        `json:"nextPage"`
	ErrorClass        string     `json:"errorClass,omitempty"`
	ErrorMessage      string     `json:"errorMessage,omitempty"`
	RateLimited       bool       `json:"rateLimited"`
	RateLimitResetAt  *time.Time `json:"rateLimitResetAt,omitempty"`
}

func toRunJSON(run tools.RunResult) runJSON {
	return runJSON{
		ID:                run.ID,
		Status:            string(run.Status),
		FinishedAt:        run.FinishedAt,
		ResultCount:       run.ResultCount,
		NewCount:          run.NewCount,
		UpdatedCount:      run.UpdatedCount,
		SkippedCount:      run.SkippedCount,
		PagesScanned:      run.PagesScanned,
		IncompleteResults: run.IncompleteResults,
		Truncated:         run.Truncated,
		PauseRequested:    run.PauseRequested,
		NextQueryIndex:    run.NextQueryIndex,
		NextPage:          run.NextPage,
		ErrorClass:        run.ErrorClass,
		ErrorMessage:      run.ErrorMessage,
		RateLimited:       run.RateLimited,
		RateLimitResetAt:  run.RateLimitResetAt,
	}
}

type githubRepoJSON struct {
	NodeID        string     `json:"nodeId"`
	Owner         string     `json:"owner"`
	Repo          string     `json:"repo"`
	FullName      string     `json:"fullName"`
	URL           string     `json:"url"`
	HomepageURL   string     `json:"homepageUrl,omitempty"`
	Name          string     `json:"name"`
	Description   string     `json:"description,omitempty"`
	Summary       string     `json:"summary,omitempty"`
	Stars         int        `json:"stars"`
	Forks         int        `json:"forks"`
	OpenIssues    int        `json:"openIssues"`
	Topics        []string   `json:"topics"`
	LicenseSPDX   string     `json:"licenseSpdx,omitempty"`
	DefaultBranch string     `json:"defaultBranch,omitempty"`
	PushedAt      *time.Time `json:"pushedAt,omitempty"`
}

func toGitHubRepoJSON(repo tools.GitHubRepo) githubRepoJSON {
	topics := repo.Topics
	if topics == nil {
		topics = []string{}
	}
	return githubRepoJSON{
		NodeID:        repo.NodeID,
		Owner:         repo.Owner,
		Repo:          repo.Repo,
		FullName:      repo.FullName,
		URL:           repo.URL,
		HomepageURL:   repo.HomepageURL,
		Name:          repo.Name,
		Description:   repo.Description,
		Summary:       repo.Summary,
		Stars:         repo.Stars,
		Forks:         repo.Forks,
		OpenIssues:    repo.OpenIssues,
		Topics:        topics,
		LicenseSPDX:   repo.LicenseSPDX,
		DefaultBranch: repo.DefaultBranch,
		PushedAt:      repo.PushedAt,
	}
}

func toToolItemJSON(item tools.ToolListItem) toolItemJSON {
	return toToolJSON(item.Tool, item.Sources, item.Evaluation).withStars7D(item.Stars7D)
}

func toToolJSON(tool tools.Tool, sources []tools.ToolSourceSummary, eval *tools.EvaluationSummary) toolItemJSON {
	currentSummary, currentSummarySource := currentToolSummary(tool)
	out := toolItemJSON{
		ID:                     tool.ID,
		Name:                   tool.Name,
		GitHubNodeID:           tool.GitHubNodeID,
		GitHubOwner:            tool.GitHubOwner,
		GitHubRepo:             tool.GitHubRepo,
		GitHubFullName:         tool.GitHubFullName,
		GitHubURL:              tool.GitHubURL,
		HomepageURL:            tool.HomepageURL,
		Description:            tool.Description,
		TemporarySummary:       tool.TemporarySummary,
		TemporarySummarySource: tool.TemporarySummarySource,
		FinalSummary:           tool.FinalSummary,
		CurrentSummary:         currentSummary,
		CurrentSummarySource:   currentSummarySource,
		Stars:                  tool.Stars,
		Forks:                  tool.Forks,
		OpenIssues:             tool.OpenIssues,
		Topics:                 tool.Topics,
		LicenseSPDX:            tool.LicenseSPDX,
		DefaultBranch:          tool.DefaultBranch,
		PushedAt:               tool.PushedAt,
		Status:                 string(tool.Status),
		FirstDiscoveredAt:      tool.FirstDiscoveredAt,
		LastDiscoveredAt:       tool.LastDiscoveredAt,
		IncludedAt:             tool.IncludedAt,
		ExcludedAt:             tool.ExcludedAt,
		ExcludedStage:          tool.ExcludedStage,
		ExcludedReason:         tool.ExcludedReason,
		StatusVersion:          tool.StatusVersion,
		SummaryKey:             toolURLKey(tool.GitHubURL),
		Sources:                make([]toolSourceJSON, 0, len(sources)),
		CreatedAt:              tool.CreatedAt,
		UpdatedAt:              tool.UpdatedAt,
	}
	if out.Topics == nil {
		out.Topics = []string{}
	}
	for _, source := range sources {
		out.Sources = append(out.Sources, toolSourceJSON{
			SourceType: string(source.SourceType),
			Term:       source.Term,
			ConfigID:   source.ConfigID,
			LastSeenAt: source.LastSeenAt,
			HitCount:   source.HitCount,
		})
	}
	if eval != nil {
		out.Evaluation = toEvaluationSummaryJSON(eval)
	}
	return out
}

func (t toolItemJSON) withStars7D(stars7D *int) toolItemJSON {
	t.Stars7D = stars7D
	return t
}

func currentToolSummary(tool tools.Tool) (*string, string) {
	if tool.FinalSummary != nil && strings.TrimSpace(*tool.FinalSummary) != "" {
		return tool.FinalSummary, "final"
	}
	if tool.TemporarySummary != nil && strings.TrimSpace(*tool.TemporarySummary) != "" {
		return tool.TemporarySummary, "temporary"
	}
	if tool.Description != nil && strings.TrimSpace(*tool.Description) != "" {
		return tool.Description, "github_description"
	}
	return nil, "none"
}

func matchesConfigFilters(cfg tools.DiscoveryConfig, methods []tools.DiscoveryMethod, triggers []tools.TriggerMode) bool {
	if len(methods) > 0 {
		ok := false
		for _, method := range methods {
			if cfg.Method == method {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(triggers) > 0 {
		ok := false
		for _, trigger := range triggers {
			if cfg.TriggerMode == trigger {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func toEvaluationSummaryJSON(eval *tools.EvaluationSummary) *evaluationSummaryJSON {
	if eval == nil {
		return nil
	}
	var result *string
	if eval.Result != nil {
		v := string(*eval.Result)
		result = &v
	}
	return &evaluationSummaryJSON{
		ID:                eval.ID,
		Evaluator:         eval.Evaluator,
		Operator:          eval.Operator,
		CooperURL:         eval.CooperURL,
		StartedAt:         eval.StartedAt,
		CompletedAt:       eval.CompletedAt,
		Result:            result,
		FinalSummary:      eval.FinalSummary,
		NotIncludedReason: eval.NotIncludedReason,
	}
}

func toEvaluationJSON(eval tools.Evaluation) evaluationSummaryJSON {
	var result *string
	if eval.Result != nil {
		v := string(*eval.Result)
		result = &v
	}
	return evaluationSummaryJSON{
		ID:                eval.ID,
		Evaluator:         eval.Evaluator,
		Operator:          eval.Operator,
		CooperURL:         eval.CooperURL,
		StartedAt:         eval.StartedAt,
		CompletedAt:       eval.CompletedAt,
		Result:            result,
		FinalSummary:      eval.FinalSummary,
		NotIncludedReason: eval.NotIncludedReason,
	}
}

func accumulateToolStats(stats *toolListStats, item toolItemJSON, evaluatorSeen map[string]bool) {
	sourceSeen := map[string]bool{}
	for _, source := range item.Sources {
		sourceSeen[source.SourceType] = true
	}
	if sourceSeen[string(tools.SourceKeyword)] {
		stats.KeywordSourceCount++
	}
	if sourceSeen[string(tools.SourceTopic)] {
		stats.TopicSourceCount++
	}
	if sourceSeen[string(tools.SourceManual)] {
		stats.ManualSourceCount++
	}
	if item.Evaluation != nil {
		if item.Evaluation.CooperURL != nil {
			stats.LinkedEvaluationCount++
		} else {
			stats.UnlinkedEvaluationCount++
		}
	}
	if item.Evaluation != nil && item.Evaluation.Evaluator != "" {
		evaluatorSeen[item.Evaluation.Evaluator] = true
	}
	stats.EvaluatorCount = len(evaluatorSeen)
	if stats.LatestUpdatedAt == nil || item.UpdatedAt.After(*stats.LatestUpdatedAt) {
		t := item.UpdatedAt
		stats.LatestUpdatedAt = &t
	}
}

func parseDiscoveryMethod(raw string) (tools.DiscoveryMethod, error) {
	switch strings.TrimSpace(raw) {
	case string(tools.DiscoveryKeyword):
		return tools.DiscoveryKeyword, nil
	case string(tools.DiscoveryTopic):
		return tools.DiscoveryTopic, nil
	default:
		return "", &tools.DiscoveryError{Class: tools.ErrorValidation, Message: "method must be keyword or topic"}
	}
}

func parseTriggerMode(raw string) (tools.TriggerMode, error) {
	if strings.TrimSpace(raw) == "" {
		return tools.TriggerManual, nil
	}
	switch strings.TrimSpace(raw) {
	case string(tools.TriggerManual):
		return tools.TriggerManual, nil
	case string(tools.TriggerWeekly):
		return tools.TriggerWeekly, nil
	default:
		return "", &tools.DiscoveryError{Class: tools.ErrorValidation, Message: "triggerMode must be manual or weekly"}
	}
}

func parseOptionalDiscoveryMethods(raw []string) ([]tools.DiscoveryMethod, error) {
	values := splitToolQueryValues(raw)
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]tools.DiscoveryMethod, 0, len(values))
	seen := map[tools.DiscoveryMethod]bool{}
	for _, value := range values {
		method, err := parseDiscoveryMethod(value)
		if err != nil {
			return nil, err
		}
		if !seen[method] {
			seen[method] = true
			out = append(out, method)
		}
	}
	return out, nil
}

func parseOptionalTriggerModes(raw []string) ([]tools.TriggerMode, error) {
	values := splitToolQueryValues(raw)
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]tools.TriggerMode, 0, len(values))
	seen := map[tools.TriggerMode]bool{}
	for _, value := range values {
		trigger, err := parseTriggerMode(value)
		if err != nil {
			return nil, err
		}
		if !seen[trigger] {
			seen[trigger] = true
			out = append(out, trigger)
		}
	}
	return out, nil
}

func parseToolStatus(raw string) (tools.ToolStatus, error) {
	if strings.TrimSpace(raw) == "" {
		return tools.ToolDiscovered, nil
	}
	switch strings.TrimSpace(raw) {
	case string(tools.ToolDiscovered):
		return tools.ToolDiscovered, nil
	case string(tools.ToolEvaluating):
		return tools.ToolEvaluating, nil
	case string(tools.ToolIncluded):
		return tools.ToolIncluded, nil
	case string(tools.ToolExcluded):
		return tools.ToolExcluded, nil
	default:
		return "", &tools.DiscoveryError{Class: tools.ErrorValidation, Message: "status is invalid"}
	}
}

func parseToolSort(raw string) (tools.SortMode, error) {
	if strings.TrimSpace(raw) == "" {
		return tools.SortLatest, nil
	}
	switch strings.TrimSpace(raw) {
	case string(tools.SortLatest):
		return tools.SortLatest, nil
	case string(tools.SortStars):
		return tools.SortStars, nil
	case string(tools.SortStars7D):
		return tools.SortStars7D, nil
	default:
		return "", &tools.DiscoveryError{Class: tools.ErrorValidation, Message: "sort is invalid"}
	}
}

func parseOptionalSourceType(raw string) (*tools.SourceType, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var source tools.SourceType
	switch strings.TrimSpace(raw) {
	case string(tools.SourceKeyword):
		source = tools.SourceKeyword
	case string(tools.SourceTopic):
		source = tools.SourceTopic
	case string(tools.SourceManual):
		source = tools.SourceManual
	default:
		return nil, &tools.DiscoveryError{Class: tools.ErrorValidation, Message: "source is invalid"}
	}
	return &source, nil
}

func parseOptionalSourceTypes(raw []string) ([]tools.SourceType, error) {
	values := splitToolQueryValues(raw)
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]tools.SourceType, 0, len(values))
	seen := map[tools.SourceType]bool{}
	for _, value := range values {
		source, err := parseOptionalSourceType(value)
		if err != nil {
			return nil, err
		}
		if source == nil || seen[*source] {
			continue
		}
		seen[*source] = true
		out = append(out, *source)
	}
	return out, nil
}

func splitToolQueryValues(raw []string) []string {
	out := []string{}
	for _, value := range raw {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func parseEvaluationResult(raw string) (tools.EvaluationResult, error) {
	switch strings.TrimSpace(raw) {
	case string(tools.EvaluationIncluded):
		return tools.EvaluationIncluded, nil
	case string(tools.EvaluationExcluded):
		return tools.EvaluationExcluded, nil
	default:
		return "", &tools.DiscoveryError{Class: tools.ErrorValidation, Message: "result must be included or excluded"}
	}
}

func parseToolLimit(raw string, fallback, max int) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 || n > max {
		return 0, &tools.DiscoveryError{Class: tools.ErrorValidation, Message: fmt.Sprintf("take must be an integer 1-%d", max)}
	}
	return n, nil
}

func parseToolOffset(rawOffset, rawPage string, limit int) (int, int, error) {
	rawOffset = strings.TrimSpace(rawOffset)
	rawPage = strings.TrimSpace(rawPage)
	if rawOffset != "" {
		offset, err := strconv.Atoi(rawOffset)
		if err != nil || offset < 0 {
			return 0, 0, &tools.DiscoveryError{Class: tools.ErrorValidation, Message: "offset must be a non-negative integer"}
		}
		return offset, offset/limit + 1, nil
	}
	if rawPage == "" {
		return 0, 1, nil
	}
	page, err := strconv.Atoi(rawPage)
	if err != nil || page < 1 {
		return 0, 0, &tools.DiscoveryError{Class: tools.ErrorValidation, Message: "page must be an integer greater than 0"}
	}
	return (page - 1) * limit, page, nil
}

func normalizeConfigTerms(method tools.DiscoveryMethod, terms []string) ([]string, error) {
	if len(terms) == 0 {
		return nil, &tools.DiscoveryError{Class: tools.ErrorValidation, Message: "terms are required"}
	}
	if len(terms) > 20 {
		return nil, &tools.DiscoveryError{Class: tools.ErrorValidation, Message: "terms cannot exceed 20 items"}
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.Join(strings.Fields(strings.TrimSpace(term)), " ")
		if term == "" {
			continue
		}
		if len([]rune(term)) > 80 {
			return nil, &tools.DiscoveryError{Class: tools.ErrorValidation, Message: "term is too long"}
		}
		switch method {
		case tools.DiscoveryKeyword:
			if strings.Contains(term, ":") {
				return nil, &tools.DiscoveryError{Class: tools.ErrorValidation, Message: "keyword terms cannot contain advanced GitHub qualifiers"}
			}
		case tools.DiscoveryTopic:
			term = strings.TrimPrefix(strings.ToLower(term), "topic:")
			if !validTopicTerm(term) {
				return nil, &tools.DiscoveryError{Class: tools.ErrorValidation, Message: "topic must use lowercase letters, numbers and hyphens"}
			}
		}
		key := strings.ToLower(term)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, term)
	}
	if len(out) == 0 {
		return nil, &tools.DiscoveryError{Class: tools.ErrorValidation, Message: "terms are required"}
	}
	return out, nil
}

func validTopicTerm(term string) bool {
	if term == "" || strings.HasPrefix(term, "-") || strings.HasSuffix(term, "-") {
		return false
	}
	for _, r := range term {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func requireToolText(raw, field string, max int) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", &tools.DiscoveryError{Class: tools.ErrorValidation, Message: field + " is required"}
	}
	if len([]rune(v)) > max {
		return "", &tools.DiscoveryError{Class: tools.ErrorValidation, Message: field + " is too long"}
	}
	return v, nil
}

func resolveConfigActor(raw string, cfg *tools.DiscoveryConfig) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" && cfg != nil {
		v = strings.TrimSpace(cfg.UpdatedBy)
		if v == "" {
			v = strings.TrimSpace(cfg.CreatedBy)
		}
	}
	return requireToolText(v, "actor", 80)
}

func nullableToolQuery(raw string) *string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return nil
	}
	return &v
}

func newToolAPIID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}
