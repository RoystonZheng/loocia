CREATE TABLE IF NOT EXISTS item_terms (
    item_id TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    term    TEXT NOT NULL,
    kind    TEXT NOT NULL CHECK (kind IN ('entity','topic')),
    PRIMARY KEY (item_id, term)
);

CREATE INDEX IF NOT EXISTS item_terms_term_idx ON item_terms (term);
