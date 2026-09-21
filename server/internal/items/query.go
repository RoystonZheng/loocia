package items

import (
	"context"
	"time"
)

// listColumns mirrors itemColumns but never reads the item's full body: the
// listing never returns it (the public DTO omits body per the openapi contract —
// full text is served only by the SSR permalink page — and no List caller reads
// Item.Body). Selecting a constant in body's slot means Postgres never detoasts
// the (potentially multi-KB) body for the ~N rows a page reads, while keeping the
// column order/count identical so scanItem is unchanged (NULL scans into *string
// Body as nil). Body is still fetched by GetByID, which uses itemColumns.
const listColumns = `id, title, title_en, url, permalink, source, source_kind,
	published_at, timeline_at, summary, NULL::text AS body, category,
	score, ai_relevance, ai_selected, selected, cluster_id, duplicate_of_id, present, cluster_primary,
	image_url, video_url, NULL::text AS reason, NULL::text AS body_cn, NULL::text AS body_cn_model`

// Cursor is a typed keyset position (the sort value + id of the last row seen).
// The opaque wire encoding lives in the API layer (P1.4).
type Cursor struct {
	SortKey time.Time
	ID      string
}

// ListParams filters the public listing. Nil pointer = no constraint.
// present=true AND duplicate_of_id IS NULL are always enforced.
type ListParams struct {
	Selected   *bool
	Category   *string
	SourceKind *string    // "rss" | "html" | "mp" | "aihot"; nil = all sources
	Since      *time.Time // published_at, falling back to created_at when missing
	Until      *time.Time // exclusive upper bound on published_at/created_at
	Q          *string
	ScoreMin   *int
	After      *Cursor
	Limit      int
}

// List returns items ordered by COALESCE(published_at,epoch) DESC, id DESC.
// Freshness filters fall back to created_at when published_at is missing so
// successfully ingested items remain discoverable.
func (s *Store) List(ctx context.Context, p ListParams) ([]Item, error) {
	limit := p.Limit
	if limit <= 0 {
		limit = 50
	}
	var afterKey *time.Time
	var afterID *string
	if p.After != nil {
		k := p.After.SortKey
		afterKey = &k
		afterID = &p.After.ID
	}

	rows, err := s.pool.Query(ctx, `
		SELECT `+listColumns+`
		FROM items
		WHERE present = true
		  AND duplicate_of_id IS NULL
		  AND ($1::boolean IS NULL OR selected = $1)
		  AND ($1::boolean IS NOT TRUE OR cluster_id IS NULL OR cluster_primary IS TRUE)
		  AND ($2::text    IS NULL OR category = $2)
		  AND ($9::text    IS NULL OR source_kind = $9)
		  AND ($3::timestamptz IS NULL OR COALESCE(published_at, created_at) >= $3)
		  AND ($8::timestamptz IS NULL OR COALESCE(published_at, created_at) < $8)
		  AND ($4::text IS NULL OR (
		        title ILIKE '%'||$4||'%' OR title_en ILIKE '%'||$4||'%'
		     OR summary ILIKE '%'||$4||'%' OR body ILIKE '%'||$4||'%'))
		  AND ($10::int IS NULL OR score >= $10)
		  AND ($5::timestamptz IS NULL OR $6::text IS NULL OR
		       (COALESCE(published_at,'epoch'::timestamptz), id) < ($5, $6))
		ORDER BY COALESCE(published_at,'epoch'::timestamptz) DESC, id DESC
		LIMIT $7`,
		p.Selected, p.Category, p.Since, p.Q, afterKey, afterID, limit, p.Until, p.SourceKind, p.ScoreMin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
