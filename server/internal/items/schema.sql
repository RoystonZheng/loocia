CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS items (
    id              TEXT PRIMARY KEY,
    title           TEXT NOT NULL,
    title_en        TEXT,
    url             TEXT NOT NULL,
    permalink       TEXT NOT NULL,
    source          TEXT NOT NULL,
    source_kind     TEXT NOT NULL DEFAULT '',
    published_at    TIMESTAMPTZ,
    timeline_at     TIMESTAMPTZ,
    summary         TEXT,
    body            TEXT,
    body_cn         TEXT,
    body_cn_model   TEXT,
    reason          TEXT,
    image_url       TEXT,
    video_url       TEXT,
    category        TEXT,
    score           INTEGER CHECK (score BETWEEN 1 AND 5),
    ai_relevance    INTEGER CHECK (ai_relevance BETWEEN 1 AND 5),
    ai_selected     BOOLEAN,
    selected        BOOLEAN NOT NULL DEFAULT FALSE,
    cluster_id      TEXT,
    duplicate_of_id TEXT,
    cluster_primary BOOLEAN,
    present         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE items ADD COLUMN IF NOT EXISTS cluster_primary BOOLEAN;
ALTER TABLE items ADD COLUMN IF NOT EXISTS image_url TEXT;
ALTER TABLE items ADD COLUMN IF NOT EXISTS video_url TEXT;
ALTER TABLE items ADD COLUMN IF NOT EXISTS reason TEXT;
ALTER TABLE items ADD COLUMN IF NOT EXISTS body_cn TEXT;
ALTER TABLE items ADD COLUMN IF NOT EXISTS body_cn_model TEXT;

-- Source plans may define their own category slugs (for example
-- "macro-economics" or "security"). Remove the historical AI-only check when
-- upgrading an existing database; API validation still accepts safe slugs.
ALTER TABLE items DROP CONSTRAINT IF EXISTS items_category_check;

CREATE INDEX IF NOT EXISTS items_sortkey_idx
    ON items (COALESCE(published_at, 'epoch'::timestamptz) DESC, id DESC);
CREATE INDEX IF NOT EXISTS items_category_idx ON items (category);
CREATE INDEX IF NOT EXISTS items_title_trgm_idx    ON items USING gin (title gin_trgm_ops);
CREATE INDEX IF NOT EXISTS items_title_en_trgm_idx ON items USING gin (title_en gin_trgm_ops);
CREATE INDEX IF NOT EXISTS items_summary_trgm_idx  ON items USING gin (summary gin_trgm_ops);
CREATE INDEX IF NOT EXISTS items_body_trgm_idx     ON items USING gin (body gin_trgm_ops);
