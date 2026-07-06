package publicapi

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"aihot-server/internal/cluster"
)

// HotStore is the read surface the hot-topics handler needs (satisfied by *cluster.Store).
type HotStore interface {
	ListHotTopics(ctx context.Context) ([]cluster.HotTopicRow, error)
}

// hotTopic mirrors openapi HotTopic — internal clusterId/heat deliberately absent.
type hotTopic struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Permalink   string    `json:"permalink"`
	Source      string    `json:"source"`
	SourceCount int       `json:"sourceCount"`
	SourceNames []string  `json:"sourceNames"`
	LatestAt    time.Time `json:"latestAt"`
}

type hotTopicList struct {
	Count int        `json:"count"`
	Items []hotTopic `json:"items"`
}

// NewHotTopicsHandler serves GET /api/public/hot-topics.
func NewHotTopicsHandler(s HotStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rows, err := s.ListHotTopics(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		env := hotTopicList{Count: len(rows), Items: make([]hotTopic, 0, len(rows))}
		for _, row := range rows {
			env.Items = append(env.Items, hotTopic{
				ID: row.ID, Title: row.Title, URL: row.URL, Permalink: row.Permalink,
				Source: row.Source, SourceCount: row.SourceCount, SourceNames: row.SourceNames,
				LatestAt: row.LatestAt,
			})
		}
		body, err := json.Marshal(env)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "encoding error")
			return
		}
		sum := sha1.Sum(body)
		etag := fmt.Sprintf(`W/"hot-%x"`, sum[:8])
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", "public, s-maxage=300, stale-while-revalidate=300")
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
}
