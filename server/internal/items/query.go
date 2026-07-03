package items

import (
	"context"
	"time"
)

// Cursor is a typed keyset position (the sort value + id of the last row seen).
// The opaque wire encoding lives in the API layer (P1.4).
type Cursor struct {
	SortKey time.Time
	ID      string
}

// ListParams filters the public listing. Nil pointer = no constraint.
// present=true AND duplicate_of_id IS NULL are always enforced.
type ListParams struct {
	Selected *bool
	Category *string
	Since    *time.Time
	Q        *string
	After    *Cursor
	Limit    int
}

// List returns items ordered by COALESCE(published_at,epoch) DESC, id DESC.
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
		SELECT `+itemColumns+`
		FROM items
		WHERE present = true
		  AND duplicate_of_id IS NULL
		  AND ($1::boolean IS NULL OR selected = $1)
		  AND ($2::text    IS NULL OR category = $2)
		  AND ($3::timestamptz IS NULL OR published_at >= $3)
		  AND ($4::text IS NULL OR (
		        title ILIKE '%'||$4||'%' OR title_en ILIKE '%'||$4||'%'
		     OR summary ILIKE '%'||$4||'%' OR body ILIKE '%'||$4||'%'))
		  AND ($5::timestamptz IS NULL OR $6::text IS NULL OR
		       (COALESCE(published_at,'epoch'::timestamptz), id) < ($5, $6))
		ORDER BY COALESCE(published_at,'epoch'::timestamptz) DESC, id DESC
		LIMIT $7`,
		p.Selected, p.Category, p.Since, p.Q, afterKey, afterID, limit)
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
