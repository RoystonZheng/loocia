CREATE TABLE IF NOT EXISTS raw_items (
    id           TEXT PRIMARY KEY,
    source       TEXT NOT NULL,
    source_kind  TEXT NOT NULL DEFAULT '',
    source_role  TEXT NOT NULL DEFAULT 'discovery',
    url          TEXT NOT NULL,
    title        TEXT NOT NULL,
    published_at TIMESTAMPTZ,
    raw_content  TEXT,
    image_url    TEXT,
    video_url    TEXT,
    fetched_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed    BOOLEAN NOT NULL DEFAULT FALSE,
    processed_at TIMESTAMPTZ
);

ALTER TABLE raw_items ADD COLUMN IF NOT EXISTS image_url TEXT;
ALTER TABLE raw_items ADD COLUMN IF NOT EXISTS video_url TEXT;
ALTER TABLE raw_items ADD COLUMN IF NOT EXISTS source_role TEXT NOT NULL DEFAULT 'discovery';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'raw_items_source_role_check'
          AND conrelid = 'raw_items'::regclass
    ) THEN
        ALTER TABLE raw_items ADD CONSTRAINT raw_items_source_role_check
            CHECK (source_role IN ('official','professional','discovery'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS raw_items_unprocessed_idx
    ON raw_items (fetched_at) WHERE processed = false;
