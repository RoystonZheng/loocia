CREATE TABLE IF NOT EXISTS clusters (
    id              TEXT PRIMARY KEY,
    primary_item_id TEXT NOT NULL,
    source_count    INTEGER NOT NULL,
    source_names    JSONB NOT NULL,
    first_at        TIMESTAMPTZ NOT NULL,
    latest_at       TIMESTAMPTZ NOT NULL,
    heat            DOUBLE PRECISION NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
