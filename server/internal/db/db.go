package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool builds a pgx connection pool from a DSN.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	return pgxpool.New(ctx, dsn)
}
