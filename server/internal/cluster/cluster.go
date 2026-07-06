package cluster

import (
	"math"
	"time"
)

// Cluster is one same-event group over the current window.
type Cluster struct {
	ID            string // = primary item id
	PrimaryItemID string
	SourceCount   int      // distinct sources among members
	SourceNames   []string // 按首报时间序 (earliest report first)
	FirstAt       time.Time
	LatestAt      time.Time
	Heat          float64
}

// HotTopicRow is a cluster joined with its primary item's public fields,
// ordered by heat — the raw material for the /api/public/hot-topics endpoint.
type HotTopicRow struct {
	ID          string // primary item id
	Title       string
	URL         string
	Permalink   string
	Source      string
	SourceCount int
	SourceNames []string
	LatestAt    time.Time
}

// heatOf: sourceCount weighted by exponential decay — halves every 24h since
// the latest report. Deterministic (now injected).
func heatOf(sourceCount int, latestAt, now time.Time) float64 {
	return float64(sourceCount) * math.Pow(0.5, now.Sub(latestAt).Hours()/24)
}
