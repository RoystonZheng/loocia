CREATE TABLE IF NOT EXISTS raw_items (
    id           TEXT PRIMARY KEY,
    source       TEXT NOT NULL,
    source_kind  TEXT NOT NULL DEFAULT '',
    url          TEXT NOT NULL,
    title        TEXT NOT NULL,
    published_at TIMESTAMPTZ,
    raw_content  TEXT,
    fetched_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed    BOOLEAN NOT NULL DEFAULT FALSE,
    processed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS raw_items_unprocessed_idx
    ON raw_items (fetched_at) WHERE processed = false;
