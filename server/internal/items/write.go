package items

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrNotFound is returned by GetByID when no row matches.
var ErrNotFound = errors.New("items: not found")

const itemColumns = `id, title, title_en, url, permalink, source, source_kind,
	published_at, timeline_at, summary, body, category,
	score, ai_relevance, ai_selected, selected, cluster_id, duplicate_of_id, present, cluster_primary`

// Upsert inserts the item or updates every mutable column on id conflict.
func (s *Store) Upsert(ctx context.Context, it Item) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO items (`+itemColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		ON CONFLICT (id) DO UPDATE SET
			title=EXCLUDED.title, title_en=EXCLUDED.title_en, url=EXCLUDED.url,
			permalink=EXCLUDED.permalink, source=EXCLUDED.source, source_kind=EXCLUDED.source_kind,
			published_at=EXCLUDED.published_at, timeline_at=EXCLUDED.timeline_at,
			summary=EXCLUDED.summary, body=EXCLUDED.body, category=EXCLUDED.category,
			score=EXCLUDED.score, ai_relevance=EXCLUDED.ai_relevance, ai_selected=EXCLUDED.ai_selected,
			selected=EXCLUDED.selected, cluster_id=EXCLUDED.cluster_id,
			duplicate_of_id=EXCLUDED.duplicate_of_id, present=EXCLUDED.present,
			cluster_primary=EXCLUDED.cluster_primary,
			updated_at=now()`,
		it.ID, it.Title, it.TitleEN, it.URL, it.Permalink, it.Source, it.SourceKind,
		it.PublishedAt, it.TimelineAt, it.Summary, it.Body, it.Category,
		it.Score, it.AIRelevance, it.AISelected, it.Selected, it.ClusterID, it.DuplicateOfID, it.Present,
		it.ClusterPrimary)
	return err
}

// ClearClusters removes cluster assignments for items published in [since, until).
func (s *Store) ClearClusters(ctx context.Context, since, until time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE items SET cluster_id = NULL, cluster_primary = NULL
		WHERE published_at >= $1 AND published_at < $2`, since, until)
	return err
}

// AssignCluster marks one item as a member (primary or secondary) of a cluster.
func (s *Store) AssignCluster(ctx context.Context, id, clusterID string, primary bool) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE items SET cluster_id = $2, cluster_primary = $3 WHERE id = $1`, id, clusterID, primary)
	return err
}

// GetByID returns the item, or ErrNotFound.
func (s *Store) GetByID(ctx context.Context, id string) (*Item, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+itemColumns+` FROM items WHERE id=$1`, id)
	it, err := scanItem(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &it, nil
}

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanItem(r rowScanner) (Item, error) {
	var it Item
	err := r.Scan(
		&it.ID, &it.Title, &it.TitleEN, &it.URL, &it.Permalink, &it.Source, &it.SourceKind,
		&it.PublishedAt, &it.TimelineAt, &it.Summary, &it.Body, &it.Category,
		&it.Score, &it.AIRelevance, &it.AISelected, &it.Selected, &it.ClusterID, &it.DuplicateOfID, &it.Present,
		&it.ClusterPrimary)
	return it, err
}
