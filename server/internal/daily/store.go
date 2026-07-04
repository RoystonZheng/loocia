package daily

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

// ErrNotFound is returned when no report matches.
var ErrNotFound = errors.New("daily: not found")

// Store persists daily reports.
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

// Upsert stores the report, replacing any existing report for the same date.
func (s *Store) Upsert(ctx context.Context, r Report) error {
	sections, err := json.Marshal(r.Sections)
	if err != nil {
		return fmt.Errorf("marshal sections: %w", err)
	}
	flashes, err := json.Marshal(r.Flashes)
	if err != nil {
		return fmt.Errorf("marshal flashes: %w", err)
	}
	var leadTitle, leadParagraph *string
	if r.Lead != nil {
		leadTitle, leadParagraph = &r.Lead.Title, &r.Lead.LeadParagraph
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO dailies (date, generated_at, window_start, window_end, lead_title, lead_paragraph, sections, flashes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (date) DO UPDATE SET
			generated_at=EXCLUDED.generated_at, window_start=EXCLUDED.window_start,
			window_end=EXCLUDED.window_end, lead_title=EXCLUDED.lead_title,
			lead_paragraph=EXCLUDED.lead_paragraph, sections=EXCLUDED.sections, flashes=EXCLUDED.flashes`,
		r.Date, r.GeneratedAt, r.WindowStart, r.WindowEnd, leadTitle, leadParagraph, sections, flashes)
	return err
}

const reportColumns = `date, generated_at, window_start, window_end, lead_title, lead_paragraph, sections, flashes`

func (s *Store) GetLatest(ctx context.Context) (*Report, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+reportColumns+` FROM dailies ORDER BY date DESC LIMIT 1`)
	return scanReport(row)
}

func (s *Store) GetByDate(ctx context.Context, date string) (*Report, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+reportColumns+` FROM dailies WHERE date=$1`, date)
	return scanReport(row)
}

func scanReport(row pgx.Row) (*Report, error) {
	var r Report
	var leadTitle, leadParagraph *string
	var sections, flashes []byte
	err := row.Scan(&r.Date, &r.GeneratedAt, &r.WindowStart, &r.WindowEnd, &leadTitle, &leadParagraph, &sections, &flashes)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if leadTitle != nil && leadParagraph != nil {
		r.Lead = &Lead{Title: *leadTitle, LeadParagraph: *leadParagraph}
	}
	if err := json.Unmarshal(sections, &r.Sections); err != nil {
		return nil, fmt.Errorf("unmarshal sections: %w", err)
	}
	if err := json.Unmarshal(flashes, &r.Flashes); err != nil {
		return nil, fmt.Errorf("unmarshal flashes: %w", err)
	}
	return &r, nil
}

// ListRecent returns up to take newest-first archive entries.
func (s *Store) ListRecent(ctx context.Context, take int) ([]Entry, error) {
	if take <= 0 {
		take = 30
	}
	rows, err := s.pool.Query(ctx, `
		SELECT date, generated_at, lead_title, lead_paragraph
		FROM dailies ORDER BY date DESC LIMIT $1`, take)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.Date, &e.GeneratedAt, &e.LeadTitle, &e.LeadParagraph); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
