package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://llm-proxy.intra.xiaojukeji.com"
	defaultModel   = "auto-max"
	maxTokens      = 1024
)

// Client calls the internal Anthropic-compatible LLM proxy.
type Client struct {
	baseURL string
	apiKey  string
	model   string
	httpc   *http.Client
}

func NewClient(baseURL, apiKey, model string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		httpc:   &http.Client{Timeout: 120 * time.Second},
	}
}

// NewClientFromEnv reads AIHOT_LLM_API_KEY (required) and optional
// AIHOT_LLM_BASE_URL / AIHOT_LLM_MODEL overrides.
func NewClientFromEnv() (*Client, error) {
	key := os.Getenv("AIHOT_LLM_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("AIHOT_LLM_API_KEY not set")
	}
	base := os.Getenv("AIHOT_LLM_BASE_URL")
	if base == "" {
		base = defaultBaseURL
	}
	model := os.Getenv("AIHOT_LLM_MODEL")
	if model == "" {
		model = defaultModel
	}
	return NewClient(base, key, model), nil
}

type msgRequest struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	System    string       `json:"system,omitempty"`
	Messages  []msgContent `json:"messages"`
}

type msgContent struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type msgResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
}

// maxAttempts bounds retries on transient failures (shared-key 429 throttling,
// 5xx). The internal proxy's key is shared across agents, so a burst of enrich
// calls routinely crosses its parallel/RPM limit; a short backoff clears it.
const maxAttempts = 5

// baseBackoff is the first retry delay; it doubles each attempt (2s,4s,8s,16s).
// A var so tests can shrink it.
var baseBackoff = 2 * time.Second

// Complete sends one system+user turn and returns the assistant's text. It
// retries transient errors (HTTP 429 and 5xx, transport errors) with exponential
// backoff, honoring a Retry-After header when present.
func (c *Client) Complete(ctx context.Context, system, user string) (string, error) {
	reqBody, err := json.Marshal(msgRequest{
		Model:     c.model,
		MaxTokens: maxTokens,
		System:    system,
		Messages:  []msgContent{{Role: "user", Content: user}},
	})
	if err != nil {
		return "", err
	}

	var lastErr error
	var retryAfter time.Duration
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(ctx, backoff(attempt, retryAfter)); err != nil {
				return "", err
			}
		}
		text, retry, ra, err := c.attempt(ctx, reqBody)
		if err == nil {
			return text, nil
		}
		lastErr = err
		if !retry {
			return "", err
		}
		retryAfter = ra
	}
	return "", fmt.Errorf("llm exhausted %d attempts: %w", maxAttempts, lastErr)
}

// attempt does one HTTP round-trip. retry reports whether the error is transient
// (worth retrying); ra is a server-suggested delay (0 if none).
func (c *Client) attempt(ctx context.Context, reqBody []byte) (text string, retry bool, ra time.Duration, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(reqBody))
	if err != nil {
		return "", false, 0, err
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := c.httpc.Do(req)
	if err != nil {
		return "", true, 0, fmt.Errorf("llm request: %w", err) // transport error: retry
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", true, 0, fmt.Errorf("llm read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		transient := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return "", transient, parseRetryAfter(resp.Header.Get("Retry-After")),
			fmt.Errorf("llm status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var mr msgResponse
	if err := json.Unmarshal(body, &mr); err != nil {
		return "", false, 0, fmt.Errorf("llm decode: %w", err)
	}
	for _, blk := range mr.Content {
		if blk.Type == "text" {
			return blk.Text, false, 0, nil
		}
	}
	return "", false, 0, fmt.Errorf("llm response had no text block")
}

// backoff returns the delay before the given attempt (1-indexed): the
// server-suggested Retry-After when set, else exponential base*2^(attempt-1).
func backoff(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return retryAfter
	}
	d := baseBackoff
	for i := 1; i < attempt; i++ {
		d *= 2
	}
	return d
}

// parseRetryAfter reads a Retry-After delay-seconds value; 0 if absent/invalid.
func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	secs, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || secs < 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}

// sleepCtx sleeps for d unless ctx is cancelled first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
