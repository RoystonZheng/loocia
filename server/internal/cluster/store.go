package cluster

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

// Store persists the current window's clusters (fully rebuilt each pass).
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) EnsureSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, schemaSQL)
	return err
}

// ReplaceAll atomically swaps the clusters table content for the new pass.
func (s *Store) ReplaceAll(ctx context.Context, clusters []Cluster) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM clusters`); err != nil {
		return err
	}
	for _, c := range clusters {
		names, err := json.Marshal(c.SourceNames)
		if err != nil {
			return fmt.Errorf("marshal source_names: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO clusters (id, primary_item_id, source_count, source_names, first_at, latest_at, heat)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			c.ID, c.PrimaryItemID, c.SourceCount, names, c.FirstAt, c.LatestAt, c.Heat); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ListHotTopics returns clusters heat-desc joined with their primary item's
// public fields. Internal heat is used for ORDER BY only (not exposed).
func (s *Store) ListHotTopics(ctx context.Context) ([]HotTopicRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.id, i.title, i.url, i.permalink, i.source,
		       c.source_count, c.source_names, c.latest_at
		FROM clusters c
		JOIN items i ON i.id = c.primary_item_id
		ORDER BY c.heat DESC, c.latest_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HotTopicRow
	for rows.Next() {
		var r HotTopicRow
		var names []byte
		if err := rows.Scan(&r.ID, &r.Title, &r.URL, &r.Permalink, &r.Source,
			&r.SourceCount, &names, &r.LatestAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(names, &r.SourceNames); err != nil {
			return nil, fmt.Errorf("unmarshal source_names: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
