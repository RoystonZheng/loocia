package version

import (
	"encoding/json"
	"net/http"
)

// NewHandler serves the current PublicVersion as JSON.
func NewHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Current())
	})
}
