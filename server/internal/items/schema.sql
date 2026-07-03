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
    category        TEXT CHECK (category IN ('ai-models','ai-products','industry','paper','tip')),
    score           INTEGER CHECK (score BETWEEN 0 AND 100),
    ai_relevance    INTEGER CHECK (ai_relevance BETWEEN 0 AND 100),
    ai_selected     BOOLEAN,
    selected        BOOLEAN NOT NULL DEFAULT FALSE,
    cluster_id      TEXT,
    duplicate_of_id TEXT,
    present         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS items_sortkey_idx
    ON items (COALESCE(published_at, 'epoch'::timestamptz) DESC, id DESC);
CREATE INDEX IF NOT EXISTS items_category_idx ON items (category);
CREATE INDEX IF NOT EXISTS items_title_trgm_idx    ON items USING gin (title gin_trgm_ops);
CREATE INDEX IF NOT EXISTS items_title_en_trgm_idx ON items USING gin (title_en gin_trgm_ops);
CREATE INDEX IF NOT EXISTS items_summary_trgm_idx  ON items USING gin (summary gin_trgm_ops);
CREATE INDEX IF NOT EXISTS items_body_trgm_idx     ON items USING gin (body gin_trgm_ops);
