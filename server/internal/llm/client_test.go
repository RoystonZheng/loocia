package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCompleteSendsAnthropicRequestAndReturnsText(t *testing.T) {
	var gotBody map[string]any
	var gotAPIKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("x-api-key")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"hello 世界"}],"stop_reason":"end_turn"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-key", "auto-max")
	out, err := c.Complete(context.Background(), "sys prompt", "user prompt")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if out != "hello 世界" {
		t.Fatalf("text: %q", out)
	}
	if gotAPIKey != "test-key" {
		t.Fatalf("x-api-key header: %q", gotAPIKey)
	}
	if gotBody["model"] != "auto-max" {
		t.Fatalf("model in body: %v", gotBody["model"])
	}
	if gotBody["system"] != "sys prompt" {
		t.Fatalf("system in body: %v", gotBody["system"])
	}
}

func TestCompleteReturnsErrorOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"nope"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k", "auto-max")
	_, err := c.Complete(context.Background(), "", "hi")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("want error mentioning 401, got %v", err)
	}
}

func TestCompleteRetriesOn429ThenSucceeds(t *testing.T) {
	old := baseBackoff
	baseBackoff = time.Millisecond
	defer func() { baseBackoff = old }()

	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limit"}`))
			return
		}
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k", "auto-max")
	out, err := c.Complete(context.Background(), "", "hi")
	if err != nil {
		t.Fatalf("should have recovered after retries: %v", err)
	}
	if out != "ok" {
		t.Fatalf("out: %q", out)
	}
	if calls != 3 {
		t.Fatalf("want 3 attempts (2×429 + 1 ok), got %d", calls)
	}
}

func TestCompleteExhaustsRetriesOn429(t *testing.T) {
	old := baseBackoff
	baseBackoff = time.Millisecond
	defer func() { baseBackoff = old }()

	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k", "auto-max")
	_, err := c.Complete(context.Background(), "", "hi")
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("want 429 error after exhausting retries, got %v", err)
	}
	if calls != maxAttempts {
		t.Fatalf("want %d attempts, got %d", maxAttempts, calls)
	}
}

func TestCompleteDoesNotRetryOn401(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k", "auto-max")
	if _, err := c.Complete(context.Background(), "", "hi"); err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("401 must not retry, got %d calls", calls)
	}
}

// Gated real-LLM smoke: only runs when AIHOT_LLM_API_KEY is set (needs VPN).
func TestCompleteRealProxy(t *testing.T) {
	if os.Getenv("AIHOT_LLM_API_KEY") == "" {
		t.Skip("AIHOT_LLM_API_KEY not set")
	}
	c, err := NewClientFromEnv()
	if err != nil {
		t.Fatalf("NewClientFromEnv: %v", err)
	}
	out, err := c.Complete(context.Background(), "只回答一个词。", "中国的首都是哪里？只回答城市名。")
	if err != nil {
		t.Fatalf("real Complete: %v", err)
	}
	if !strings.Contains(out, "北京") {
		t.Fatalf("unexpected real answer: %q", out)
	}
}
