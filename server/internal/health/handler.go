package health

import (
	"context"
	"net/http"
)

// Pinger is anything that can check backing-store liveness.
type Pinger interface {
	Ping(ctx context.Context) error
}

func NewHandler(p Pinger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := p.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"down"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
}
