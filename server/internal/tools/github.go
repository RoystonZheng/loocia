package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	GitHubDefaultBaseURL = "https://api.github.com"

	ErrorBadQuery           = "bad_query"
	ErrorGitHub             = "github_error"
	ErrorInvalidCredentials = "invalid_credentials"
	ErrorInvalidGitHubURL   = "invalid_github_url"
	ErrorNetwork            = "network_error"
	ErrorRateLimited        = "rate_limited"
	ErrorRepositoryRejected = "repository_rejected"
	ErrorInvalidCooperURL   = "invalid_cooper_url"
	ErrorMissingCooperURL   = "missing_cooper_url"
	ErrorStatusConflict     = "status_conflict"
	ErrorTruncated          = "truncated"
	ErrorValidation         = "validation_error"
)

type DiscoveryError struct {
	Class            string
	Message          string
	RateLimited      bool
	RateLimitResetAt *time.Time
}

func (e *DiscoveryError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return e.Class
	}
	return e.Class + ": " + e.Message
}

func AsDiscoveryError(err error, target **DiscoveryError) bool {
	return errors.As(err, target)
}

type GitHubSearchRequest struct {
	Query   string
	Page    int
	PerPage int
	Sort    string
	Order   string
}

type GitHubSearchResult struct {
	Repos             []GitHubRepo
	TotalCount        int
	IncompleteResults bool
}

type HTTPGitHubClient struct {
	BaseURL                    string
	Token                      string
	DefaultTokens              []string
	Tokens                     []string
	TokenStrategy              GitHubTokenStrategy
	ActiveTokenIndex           int
	IncludeDefaultGitHubTokens bool
	Client                     *http.Client
	mu                         sync.Mutex
	next                       int
}

func NewHTTPGitHubClient(token string) *HTTPGitHubClient {
	return &HTTPGitHubClient{
		BaseURL:                    GitHubDefaultBaseURL,
		Token:                      strings.TrimSpace(token),
		DefaultTokens:              NormalizeGitHubTokens([]string{token}),
		TokenStrategy:              GitHubTokenStrategyRoundRobin,
		IncludeDefaultGitHubTokens: true,
		Client:                     &http.Client{Timeout: 30 * time.Second},
	}
}

func IsValidGitHubTokenStrategy(strategy GitHubTokenStrategy) bool {
	switch strategy {
	case GitHubTokenStrategyRoundRobin, GitHubTokenStrategyFixed, GitHubTokenStrategyFailover:
		return true
	default:
		return false
	}
}

func SplitGitHubTokens(raw string) []string {
	return NormalizeGitHubTokens(strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '，' || r == '；'
	}))
}

func NormalizeGitHubTokens(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	seen := map[string]bool{}
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" || seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	return out
}

func (c *HTTPGitHubClient) SetTokens(tokens []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Tokens = NormalizeGitHubTokens(tokens)
	c.next = 0
}

func (c *HTTPGitHubClient) SetDefaultTokens(tokens []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.DefaultTokens = NormalizeGitHubTokens(append([]string{c.Token}, tokens...))
}

func (c *HTTPGitHubClient) SetTokenSelection(strategy GitHubTokenStrategy, activeIndex int, includeDefaultTokens bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !IsValidGitHubTokenStrategy(strategy) {
		strategy = GitHubTokenStrategyRoundRobin
	}
	c.TokenStrategy = strategy
	if activeIndex < 0 {
		activeIndex = 0
	}
	c.ActiveTokenIndex = activeIndex
	c.IncludeDefaultGitHubTokens = includeDefaultTokens
}

func (c *HTTPGitHubClient) AuthTokens() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.authTokensLocked()
}

func (c *HTTPGitHubClient) authTokensLocked() []string {
	tokens := []string{}
	if c.IncludeDefaultGitHubTokens {
		tokens = append(tokens, c.DefaultTokens...)
	}
	if len(c.Tokens) > 0 {
		tokens = append(tokens, c.Tokens...)
	}
	if len(tokens) == 0 && strings.TrimSpace(c.Token) != "" {
		tokens = append(tokens, strings.TrimSpace(c.Token))
	}
	return NormalizeGitHubTokens(tokens)
}

func (c *HTTPGitHubClient) SearchRepositories(ctx context.Context, req GitHubSearchRequest) (GitHubSearchResult, error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PerPage <= 0 {
		req.PerPage = 100
	}

	values := url.Values{}
	values.Set("q", req.Query)
	values.Set("page", strconv.Itoa(req.Page))
	values.Set("per_page", strconv.Itoa(req.PerPage))
	if req.Sort != "" {
		values.Set("sort", req.Sort)
	}
	if req.Order != "" {
		values.Set("order", req.Order)
	}

	var payload githubSearchResponse
	if err := c.getJSON(ctx, "/search/repositories", values, &payload); err != nil {
		return GitHubSearchResult{}, err
	}
	out := GitHubSearchResult{
		TotalCount:        payload.TotalCount,
		IncompleteResults: payload.IncompleteResults,
		Repos:             make([]GitHubRepo, 0, len(payload.Items)),
	}
	for _, item := range payload.Items {
		out.Repos = append(out.Repos, item.toGitHubRepo())
	}
	return out, nil
}

func (c *HTTPGitHubClient) GetRepository(ctx context.Context, owner, repo string) (GitHubRepo, error) {
	path := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)
	var payload githubRepoResponse
	if err := c.getJSON(ctx, path, nil, &payload); err != nil {
		return GitHubRepo{}, err
	}
	return payload.toGitHubRepo(), nil
}

func (c *HTTPGitHubClient) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	tokens := c.orderedAuthTokens()
	if len(tokens) == 0 {
		return c.getJSONWithToken(ctx, path, query, out, "")
	}
	var lastErr error
	allInvalidCredentials := true
	for _, token := range tokens {
		err := c.getJSONWithToken(ctx, path, query, out, token)
		if err == nil {
			return nil
		}
		lastErr = err
		var derr *DiscoveryError
		if !AsDiscoveryError(err, &derr) {
			return err
		}
		if derr.Class == ErrorInvalidCredentials {
			continue
		}
		allInvalidCredentials = false
		if !derr.RateLimited {
			return err
		}
	}
	if allInvalidCredentials {
		// A revoked or expired token should not block public repository discovery.
		// Retry once without authentication; GitHub still permits public search,
		// with its lower anonymous rate limit.
		return c.getJSONWithToken(ctx, path, query, out, "")
	}
	return lastErr
}

func (c *HTTPGitHubClient) orderedAuthTokens() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	tokens := c.authTokensLocked()
	if len(tokens) == 0 {
		return nil
	}
	index := c.ActiveTokenIndex
	if index >= len(tokens) {
		index = 0
	}
	switch c.TokenStrategy {
	case GitHubTokenStrategyFixed:
		return []string{tokens[index]}
	case GitHubTokenStrategyFailover:
		return rotateTokens(tokens, index)
	default:
		start := c.nextAuthTokenIndexLocked(tokens)
		return rotateTokens(tokens, start)
	}
}

func (c *HTTPGitHubClient) nextAuthTokenIndexLocked(tokens []string) int {
	if len(tokens) == 1 {
		return 0
	}
	index := c.next % len(tokens)
	c.next = (c.next + 1) % len(tokens)
	return index
}

func rotateTokens(tokens []string, start int) []string {
	if len(tokens) == 0 {
		return nil
	}
	if start < 0 || start >= len(tokens) {
		start = 0
	}
	out := make([]string, 0, len(tokens))
	out = append(out, tokens[start:]...)
	out = append(out, tokens[:start]...)
	return out
}

type GitHubRateLimit struct {
	Limit     int
	Remaining int
	ResetAt   *time.Time
}

func (c *HTTPGitHubClient) CheckToken(ctx context.Context, token string) (GitHubRateLimit, error) {
	var payload struct {
		Rate struct {
			Limit     int   `json:"limit"`
			Remaining int   `json:"remaining"`
			Reset     int64 `json:"reset"`
		} `json:"rate"`
	}
	if err := c.getJSONWithToken(ctx, "/rate_limit", nil, &payload, strings.TrimSpace(token)); err != nil {
		return GitHubRateLimit{}, err
	}
	var resetAt *time.Time
	if payload.Rate.Reset > 0 {
		t := time.Unix(payload.Rate.Reset, 0).UTC()
		resetAt = &t
	}
	return GitHubRateLimit{Limit: payload.Rate.Limit, Remaining: payload.Rate.Remaining, ResetAt: resetAt}, nil
}

func (c *HTTPGitHubClient) getJSONWithToken(ctx context.Context, path string, query url.Values, out any, token string) error {
	base := c.BaseURL
	if base == "" {
		base = GitHubDefaultBaseURL
	}
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	u, err := url.Parse(strings.TrimRight(base, "/") + path)
	if err != nil {
		return err
	}
	if query != nil {
		u.RawQuery = query.Encode()
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Accept", "application/vnd.github+json")
	httpReq.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	httpReq.Header.Set("User-Agent", "ai-tool/0.1")
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return &DiscoveryError{Class: ErrorNetwork, Message: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return githubHTTPError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode github response: %w", err)
	}
	return nil
}

func githubHTTPError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = resp.Status
	}
	if resp.StatusCode == http.StatusUnprocessableEntity {
		return &DiscoveryError{Class: ErrorBadQuery, Message: msg}
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return &DiscoveryError{Class: ErrorInvalidCredentials, Message: msg}
	}
	if isRateLimited(resp, msg) {
		return &DiscoveryError{
			Class:            ErrorRateLimited,
			Message:          msg,
			RateLimited:      true,
			RateLimitResetAt: parseRateLimitReset(resp.Header),
		}
	}
	return &DiscoveryError{Class: ErrorGitHub, Message: msg}
}

func isRateLimited(resp *http.Response, msg string) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if resp.StatusCode != http.StatusForbidden {
		return false
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return true
	}
	return strings.Contains(strings.ToLower(msg), "rate limit")
}

func parseRateLimitReset(h http.Header) *time.Time {
	if retryAfter := h.Get("Retry-After"); retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil {
			t := time.Now().UTC().Add(time.Duration(seconds) * time.Second)
			return &t
		}
	}
	raw := h.Get("X-RateLimit-Reset")
	if raw == "" {
		return nil
	}
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil
	}
	t := time.Unix(seconds, 0).UTC()
	return &t
}

func ParseGitHubRepoURL(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", &DiscoveryError{Class: ErrorInvalidGitHubURL, Message: "empty repository URL"}
	}

	if strings.HasPrefix(raw, "git@github.com:") {
		rest := strings.TrimPrefix(raw, "git@github.com:")
		return parseGitHubPath(rest)
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", "", &DiscoveryError{Class: ErrorInvalidGitHubURL, Message: "invalid repository URL"}
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", "", &DiscoveryError{Class: ErrorInvalidGitHubURL, Message: "unsupported repository URL scheme"}
	}
	if strings.ToLower(u.Hostname()) != "github.com" {
		return "", "", &DiscoveryError{Class: ErrorInvalidGitHubURL, Message: "repository host must be github.com"}
	}
	return parseGitHubPath(u.EscapedPath())
}

func parseGitHubPath(path string) (string, string, error) {
	path = strings.Trim(path, "/")
	if path == "" {
		return "", "", &DiscoveryError{Class: ErrorInvalidGitHubURL, Message: "missing owner and repo"}
	}
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return "", "", &DiscoveryError{Class: ErrorInvalidGitHubURL, Message: "missing repo"}
	}
	owner, err := url.PathUnescape(parts[0])
	if err != nil {
		return "", "", &DiscoveryError{Class: ErrorInvalidGitHubURL, Message: "invalid owner"}
	}
	repo, err := url.PathUnescape(parts[1])
	if err != nil {
		return "", "", &DiscoveryError{Class: ErrorInvalidGitHubURL, Message: "invalid repo"}
	}
	repo = strings.TrimSuffix(repo, ".git")
	if !validGitHubSegment(owner) || !validGitHubSegment(repo) {
		return "", "", &DiscoveryError{Class: ErrorInvalidGitHubURL, Message: "invalid owner or repo"}
	}
	return owner, repo, nil
}

func validGitHubSegment(v string) bool {
	if v == "" || strings.ContainsAny(v, `/\?#:`) {
		return false
	}
	for _, r := range v {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

type githubSearchResponse struct {
	TotalCount        int                  `json:"total_count"`
	IncompleteResults bool                 `json:"incomplete_results"`
	Items             []githubRepoResponse `json:"items"`
}

type githubRepoResponse struct {
	NodeID          string    `json:"node_id"`
	Name            string    `json:"name"`
	FullName        string    `json:"full_name"`
	HTMLURL         string    `json:"html_url"`
	Homepage        string    `json:"homepage"`
	Description     string    `json:"description"`
	StargazersCount int       `json:"stargazers_count"`
	ForksCount      int       `json:"forks_count"`
	OpenIssuesCount int       `json:"open_issues_count"`
	Topics          []string  `json:"topics"`
	License         *license  `json:"license"`
	DefaultBranch   string    `json:"default_branch"`
	PushedAt        time.Time `json:"pushed_at"`
	Archived        bool      `json:"archived"`
	Fork            bool      `json:"fork"`
	Owner           owner     `json:"owner"`
}

type owner struct {
	Login string `json:"login"`
}

type license struct {
	SPDXID string `json:"spdx_id"`
}

func (r githubRepoResponse) toGitHubRepo() GitHubRepo {
	ownerLogin := r.Owner.Login
	repoName := r.Name
	if ownerLogin == "" || repoName == "" {
		parts := strings.SplitN(r.FullName, "/", 2)
		if len(parts) == 2 {
			if ownerLogin == "" {
				ownerLogin = parts[0]
			}
			if repoName == "" {
				repoName = parts[1]
			}
		}
	}
	fullName := r.FullName
	if fullName == "" && ownerLogin != "" && repoName != "" {
		fullName = ownerLogin + "/" + repoName
	}
	var pushedAt *time.Time
	if !r.PushedAt.IsZero() {
		t := r.PushedAt.UTC()
		pushedAt = &t
	}
	licenseSPDX := ""
	if r.License != nil {
		licenseSPDX = r.License.SPDXID
	}
	return GitHubRepo{
		NodeID:        r.NodeID,
		Owner:         ownerLogin,
		Repo:          repoName,
		FullName:      fullName,
		URL:           r.HTMLURL,
		HomepageURL:   r.Homepage,
		Name:          repoName,
		Description:   r.Description,
		Stars:         r.StargazersCount,
		Forks:         r.ForksCount,
		OpenIssues:    r.OpenIssuesCount,
		Topics:        r.Topics,
		LicenseSPDX:   licenseSPDX,
		DefaultBranch: r.DefaultBranch,
		PushedAt:      pushedAt,
		Archived:      r.Archived,
		Fork:          r.Fork,
	}
}
