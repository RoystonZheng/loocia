package ingest

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// newSourceHTTPClient keeps source traffic separately configurable from the
// internal LLM client. Standard HTTP(S)_PROXY variables still work through
// http.DefaultTransport; AIHOT_SOURCE_HTTP_PROXY provides an explicit
// source-only override for deployment hosts with restricted egress.
func newSourceHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if raw := strings.TrimSpace(os.Getenv("AIHOT_SOURCE_HTTP_PROXY")); raw != "" {
		if proxyURL, err := url.Parse(raw); err == nil && proxyURL.Scheme != "" && proxyURL.Host != "" {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}
