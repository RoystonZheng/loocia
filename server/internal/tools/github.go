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
	"time"
)

const (
	GitHubDefaultBaseURL = "https://api.github.com"

	ErrorBadQuery           = "bad_query"
	ErrorGitHub             = "github_error"
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
	BaseURL string
	Token   string
	Client  *http.Client
}

func NewHTTPGitHubClient(token string) *HTTPGitHubClient {
	return &HTTPGitHubClient{
		BaseURL: GitHubDefaultBaseURL,
		Token:   token,
		Client:  &http.Client{Timeout: 30 * time.Second},
	}
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
	if c.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.Token)
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
