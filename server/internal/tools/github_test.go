package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBuildGitHubQueriesUsesBaselineAndIncrementalRules(t *testing.T) {
	cfg := DiscoveryConfig{
		ID:          "cfg-keyword",
		Method:      DiscoveryKeyword,
		Terms:       []string{" coding agent "},
		TriggerMode: TriggerManual,
	}
	queries := BuildGitHubQueries(cfg)
	if len(queries) != 1 {
		t.Fatalf("baseline query count = %d", len(queries))
	}
	if queries[0].Query != `"coding agent" in:name,description,topics,readme fork:false archived:false` {
		t.Fatalf("baseline query mismatch: %q", queries[0].Query)
	}
	if queries[0].Sort != "stars" || queries[0].Order != "desc" {
		t.Fatalf("baseline sort mismatch: %+v", queries[0])
	}

	last := time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC)
	cfg.LastSuccessAt = &last
	queries = BuildGitHubQueries(cfg)
	if len(queries) != 2 {
		t.Fatalf("incremental query count = %d", len(queries))
	}
	for _, want := range []string{"created:>2026-09-09T10:00:00Z", "pushed:>2026-09-09T10:00:00Z"} {
		found := false
		for _, got := range queries {
			if strings.Contains(got.Query, want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("incremental query missing %s in %+v", want, queries)
		}
	}

	topicCfg := DiscoveryConfig{Method: DiscoveryTopic, Terms: []string{"ai-agents"}}
	topicQueries := BuildGitHubQueries(topicCfg)
	if len(topicQueries) != 1 || topicQueries[0].Query != "topic:ai-agents fork:false archived:false" {
		t.Fatalf("topic query mismatch: %+v", topicQueries)
	}
}

func TestParseGitHubRepoURL(t *testing.T) {
	tests := []struct {
		in    string
		owner string
		repo  string
	}{
		{"https://github.com/openai/agents", "openai", "agents"},
		{"https://github.com/openai/agents.git?tab=readme#usage", "openai", "agents"},
		{"https://github.com/openai/agents/tree/main/examples", "openai", "agents"},
		{"git@github.com:openai/agents.git", "openai", "agents"},
	}
	for _, tt := range tests {
		owner, repo, err := ParseGitHubRepoURL(tt.in)
		if err != nil {
			t.Fatalf("ParseGitHubRepoURL(%q): %v", tt.in, err)
		}
		if owner != tt.owner || repo != tt.repo {
			t.Fatalf("ParseGitHubRepoURL(%q) = %s/%s", tt.in, owner, repo)
		}
	}

	for _, bad := range []string{"https://gitlab.com/openai/agents", "https://github.com/openai", "not a url"} {
		if _, _, err := ParseGitHubRepoURL(bad); err == nil {
			t.Fatalf("ParseGitHubRepoURL(%q) should fail", bad)
		}
	}
}

func TestHTTPGitHubClientSearchHeadersAndErrorClassification(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/repositories" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("q"); got != "topic:ai-agents fork:false archived:false" {
			t.Fatalf("query = %q", got)
		}
		if got := r.URL.Query().Get("per_page"); got != "100" {
			t.Fatalf("per_page = %q", got)
		}
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_count":        1,
			"incomplete_results": true,
			"items": []map[string]any{{
				"node_id":           "node-1",
				"name":              "agents",
				"full_name":         "openai/agents",
				"html_url":          "https://github.com/openai/agents",
				"homepage":          "https://example.com/agents",
				"description":       "Agent toolkit",
				"stargazers_count":  123,
				"forks_count":       4,
				"open_issues_count": 5,
				"topics":            []string{"ai-agents"},
				"license":           map[string]any{"spdx_id": "MIT"},
				"default_branch":    "main",
				"pushed_at":         "2026-09-10T00:00:00Z",
				"archived":          false,
				"fork":              false,
				"owner":             map[string]any{"login": "openai"},
			}},
		})
	}))
	defer srv.Close()

	client := NewHTTPGitHubClient("token-1")
	client.BaseURL = srv.URL
	result, err := client.SearchRepositories(context.Background(), GitHubSearchRequest{
		Query:   "topic:ai-agents fork:false archived:false",
		Page:    1,
		PerPage: 100,
	})
	if err != nil {
		t.Fatalf("SearchRepositories: %v", err)
	}
	if gotAuth != "Bearer token-1" {
		t.Fatalf("authorization header = %q", gotAuth)
	}
	if !result.IncompleteResults || result.TotalCount != 1 || len(result.Repos) != 1 {
		t.Fatalf("result mismatch: %+v", result)
	}
	if result.Repos[0].NodeID != "node-1" || result.Repos[0].Stars != 123 {
		t.Fatalf("repo mismatch: %+v", result.Repos[0])
	}

	resetAt := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	rateLimit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1789041600")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer rateLimit.Close()

	limited := NewHTTPGitHubClient("")
	limited.BaseURL = rateLimit.URL
	_, err = limited.SearchRepositories(context.Background(), GitHubSearchRequest{Query: "x", Page: 1, PerPage: 100})
	if err == nil {
		t.Fatal("rate limit should return error")
	}
	var derr *DiscoveryError
	if !AsDiscoveryError(err, &derr) || !derr.RateLimited || derr.Class != ErrorRateLimited {
		t.Fatalf("rate limit classification mismatch: %#v", err)
	}
	if derr.RateLimitResetAt == nil || !derr.RateLimitResetAt.Equal(resetAt) {
		t.Fatalf("reset time mismatch: %+v", derr.RateLimitResetAt)
	}

	secondaryLimit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"You have exceeded a secondary rate limit. Please wait a few minutes before you try again."}`))
	}))
	defer secondaryLimit.Close()

	secondaryLimited := NewHTTPGitHubClient("token-1")
	secondaryLimited.BaseURL = secondaryLimit.URL
	_, err = secondaryLimited.SearchRepositories(context.Background(), GitHubSearchRequest{Query: "x", Page: 1, PerPage: 100})
	if err == nil {
		t.Fatal("secondary rate limit should return error")
	}
	if !AsDiscoveryError(err, &derr) || !derr.RateLimited || derr.Class != ErrorRateLimited {
		t.Fatalf("secondary rate limit classification mismatch: %#v", err)
	}
}

func TestHTTPGitHubClientRotatesConfiguredTokens(t *testing.T) {
	var authHeaders []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_count":        0,
			"incomplete_results": false,
			"items":              []map[string]any{},
		})
	}))
	defer srv.Close()

	client := NewHTTPGitHubClient("")
	client.BaseURL = srv.URL
	client.SetTokens([]string{"token-a", "token-b"})
	client.SetTokenSelection(GitHubTokenStrategyRoundRobin, 0, false)

	for i := 0; i < 2; i++ {
		if _, err := client.SearchRepositories(context.Background(), GitHubSearchRequest{Query: "x"}); err != nil {
			t.Fatalf("SearchRepositories(%d): %v", i, err)
		}
	}
	if got, want := strings.Join(authHeaders, ","), "Bearer token-a,Bearer token-b"; got != want {
		t.Fatalf("authorization rotation = %q, want %q", got, want)
	}
}
