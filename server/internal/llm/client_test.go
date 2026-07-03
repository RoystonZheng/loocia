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
