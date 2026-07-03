package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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

// Complete sends one system+user turn and returns the assistant's text.
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := c.httpc.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("llm read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var mr msgResponse
	if err := json.Unmarshal(body, &mr); err != nil {
		return "", fmt.Errorf("llm decode: %w", err)
	}
	for _, blk := range mr.Content {
		if blk.Type == "text" {
			return blk.Text, nil
		}
	}
	return "", fmt.Errorf("llm response had no text block")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
