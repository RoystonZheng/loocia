package version

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVersionHandlerReturnsJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	NewHandler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/public/version", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("want application/json, got %q", ct)
	}
	var got PublicVersion
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not valid PublicVersion JSON: %v", err)
	}
	if got.APIVersion != "1.1.0" {
		t.Fatalf("want apiVersion 1.1.0, got %q", got.APIVersion)
	}
}
