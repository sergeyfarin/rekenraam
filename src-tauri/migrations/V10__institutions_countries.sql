CREATE TABLE IF NOT EXISTS countries (
  id         INTEGER PRIMARY KEY,
  book_id    INTEGER NOT NULL,
  code       TEXT NOT NULL,
  name       TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  UNIQUE(book_id, code),
  UNIQUE(book_id, name)
);

CREATE TABLE IF NOT EXISTS institutions (
  id         INTEGER PRIMARY KEY,
  book_id    INTEGER NOT NULL,
  name       TEXT NOT NULL,
  kind       TEXT NOT NULL DEFAULT 'other' CHECK (kind IN ('bank','broker','credit_union','other')),
  country_id INTEGER,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(country_id) REFERENCES countries(id) ON DELETE SET NULL,
  UNIQUE(book_id, name)
);

ALTER TABLE accounts ADD COLUMN institution_id INTEGER REFERENCES institutions(id);
ALTER TABLE accounts ADD COLUMN country_id INTEGER REFERENCES countries(id);

CREATE INDEX IF NOT EXISTS idx_countries_book ON countries(book_id);
CREATE INDEX IF NOT EXISTS idx_institutions_book ON institutions(book_id);
CREATE INDEX IF NOT EXISTS idx_accounts_institution ON accounts(institution_id);
CREATE INDEX IF NOT EXISTS idx_accounts_country ON accounts(country_id);
