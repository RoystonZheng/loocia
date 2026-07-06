package ingest

import (
	"context"
	_ "embed"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

// RawStore persists raw items in the staging table.
type RawStore struct {
	pool *pgxpool.Pool
}

func NewRawStore(pool *pgxpool.Pool) *RawStore {
	return &RawStore{pool: pool}
}

// EnsureSchema applies schema.sql idempotently (simple protocol: multi-statement).
func (s *RawStore) EnsureSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, schemaSQL)
	return err
}

// InsertRaw inserts a raw item, ignoring conflicts on id (idempotent — never
// overwrites an already-fetched row). Returns true if a new row was inserted.
func (s *RawStore) InsertRaw(ctx context.Context, r RawItem) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO raw_items (id, source, source_kind, url, title, published_at, raw_content, image_url)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (id) DO NOTHING`,
		r.ID, r.Source, r.SourceKind, r.URL, r.Title, r.PublishedAt, r.RawContent, r.ImageURL)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// ListUnprocessed returns up to limit oldest-first unprocessed raw items.
func (s *RawStore) ListUnprocessed(ctx context.Context, limit int) ([]RawItem, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, source, source_kind, url, title, published_at, raw_content, image_url
		FROM raw_items
		WHERE processed = false
		ORDER BY fetched_at ASC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RawItem
	for rows.Next() {
		var r RawItem
		if err := rows.Scan(&r.ID, &r.Source, &r.SourceKind, &r.URL, &r.Title, &r.PublishedAt, &r.RawContent, &r.ImageURL); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// MarkProcessed flags a raw item as consumed by the pipeline.
func (s *RawStore) MarkProcessed(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE raw_items SET processed = true, processed_at = now() WHERE id = $1`, id)
	return err
}
