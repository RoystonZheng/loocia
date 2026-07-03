package db

import (
	"context"
	"os"
	"testing"
)

func TestNewPoolPingsDatabase(t *testing.T) {
	dsn := os.Getenv("AIHOT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AIHOT_TEST_DATABASE_URL not set")
	}
	pool, err := NewPool(context.Background(), dsn)
	if err != nil {
		t.Fatalf("NewPool returned error: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}
