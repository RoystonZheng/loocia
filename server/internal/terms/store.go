// Package terms stores per-item extracted terms (entities + topics) and serves
// the aggregations behind the graph view: word-cloud counts, co-occurrence
// neighbors, and per-term item lists. All queries are real-time SQL — at the
// current scale (~1k items) precomputation would be waste. Reads exclude
// duplicate reports (duplicate_of_id IS NULL), matching the feed's semantics.
package terms

import (
	"context"
	_ "embed"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

// Term is one extracted term of an item.
type Term struct {
	Term string
	Kind string // 'entity' | 'topic'
}

// CloudTerm is one word-cloud entry: term + distinct-item count in the window.
type CloudTerm struct {
	Term  string
	Kind  string
	Count int
}

// Neighbor is one co-occurrence edge endpoint: items where both terms appear
// count 1 each, or 2 when the item belongs to a hot cluster (same-event boost).
type Neighbor struct {
	Term   string
	Kind   string
	Weight int
}

// TermItem is the public projection of an item carrying a term (the same
// pattern as cluster.HotTopicRow: a local row struct keeps packages decoupled).
type TermItem struct {
	ID          string
	Title       string
	TitleEN     *string
	URL         string
	Permalink   string
	Source      string
	PublishedAt *time.Time
	Summary     *string
	ImageURL    *string
	Category    *string
	Score       *int
	Selected    bool
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) EnsureSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, schemaSQL)
	return err
}

// ReplaceForItem swaps an item's terms atomically (delete + insert in one tx),
// so re-extraction never leaves stale rows behind.
func (s *Store) ReplaceForItem(ctx context.Context, itemID string, ts []Term) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM item_terms WHERE item_id = $1`, itemID); err != nil {
		return err
	}
	for _, t := range ts {
		if _, err := tx.Exec(ctx,
			`INSERT INTO item_terms (item_id, term, kind) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
			itemID, t.Term, t.Kind); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Cloud returns the top terms by distinct-item count since `since` (nil = all
// time). MIN(kind) prefers 'entity' when the same term was tagged both ways.
func (s *Store) Cloud(ctx context.Context, since *time.Time, limit int) ([]CloudTerm, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.term, MIN(t.kind), COUNT(*)::int AS cnt
		FROM item_terms t
		JOIN items i ON i.id = t.item_id
		WHERE i.present AND i.duplicate_of_id IS NULL
		  AND ($1::timestamptz IS NULL OR COALESCE(i.published_at,'epoch'::timestamptz) >= $1)
		GROUP BY t.term
		ORDER BY cnt DESC, t.term
		LIMIT $2`, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CloudTerm
	for rows.Next() {
		var c CloudTerm
		if err := rows.Scan(&c.Term, &c.Kind, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Neighbors returns terms co-occurring with `term` (same item), weight-desc.
// An item inside a hot cluster (same-event, multi-source) counts double.
func (s *Store) Neighbors(ctx context.Context, term string, since *time.Time, limit int) ([]Neighbor, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT b.term, MIN(b.kind),
		       SUM(CASE WHEN i.cluster_id IS NOT NULL THEN 2 ELSE 1 END)::int AS weight
		FROM item_terms a
		JOIN item_terms b ON b.item_id = a.item_id AND b.term <> a.term
		JOIN items i ON i.id = a.item_id
		WHERE a.term = $1 AND i.present AND i.duplicate_of_id IS NULL
		  AND ($2::timestamptz IS NULL OR COALESCE(i.published_at,'epoch'::timestamptz) >= $2)
		GROUP BY b.term
		ORDER BY weight DESC, b.term
		LIMIT $3`, term, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Neighbor
	for rows.Next() {
		var n Neighbor
		if err := rows.Scan(&n.Term, &n.Kind, &n.Weight); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ItemsForTerm returns the newest items carrying the term, public fields only.
func (s *Store) ItemsForTerm(ctx context.Context, term string, since *time.Time, limit int) ([]TermItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.id, i.title, i.title_en, i.url, i.permalink, i.source, i.published_at,
		       i.summary, i.image_url, i.category, i.score, i.selected
		FROM item_terms t
		JOIN items i ON i.id = t.item_id
		WHERE t.term = $1 AND i.present AND i.duplicate_of_id IS NULL
		  AND ($2::timestamptz IS NULL OR COALESCE(i.published_at,'epoch'::timestamptz) >= $2)
		ORDER BY COALESCE(i.published_at,'epoch'::timestamptz) DESC, i.id DESC
		LIMIT $3`, term, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TermItem
	for rows.Next() {
		var it TermItem
		if err := rows.Scan(&it.ID, &it.Title, &it.TitleEN, &it.URL, &it.Permalink, &it.Source,
			&it.PublishedAt, &it.Summary, &it.ImageURL, &it.Category, &it.Score, &it.Selected); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// TermInfo returns a term's kind + item count in the window; ("", 0, nil) when
// the term doesn't appear at all (the handler still answers 200 with empties).
func (s *Store) TermInfo(ctx context.Context, term string, since *time.Time) (kind string, count int, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT COALESCE(MIN(t.kind), ''), COUNT(*)::int
		FROM item_terms t
		JOIN items i ON i.id = t.item_id
		WHERE t.term = $1 AND i.present AND i.duplicate_of_id IS NULL
		  AND ($2::timestamptz IS NULL OR COALESCE(i.published_at,'epoch'::timestamptz) >= $2)`,
		term, since).Scan(&kind, &count)
	return kind, count, err
}
