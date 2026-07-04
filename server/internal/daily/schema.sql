CREATE TABLE IF NOT EXISTS dailies (
    date           TEXT PRIMARY KEY,  -- YYYY-MM-DD; ISO text orders correctly
    generated_at   TIMESTAMPTZ NOT NULL,
    window_start   TIMESTAMPTZ NOT NULL,
    window_end     TIMESTAMPTZ NOT NULL,
    lead_title     TEXT,
    lead_paragraph TEXT,
    sections       JSONB NOT NULL,
    flashes        JSONB NOT NULL
);
