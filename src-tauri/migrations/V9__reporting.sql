CREATE TABLE IF NOT EXISTS report_definitions (
  id            INTEGER PRIMARY KEY,
  book_id       INTEGER NOT NULL,
  name          TEXT NOT NULL,
  kind          TEXT NOT NULL DEFAULT 'custom' CHECK (kind IN ('builtin','custom')),
  query_type    TEXT NOT NULL DEFAULT 'sql' CHECK (query_type IN ('sql','template')),
  query_text    TEXT NOT NULL,
  params_schema TEXT,
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  UNIQUE(book_id, name)
);

CREATE INDEX IF NOT EXISTS idx_report_definitions_book ON report_definitions(book_id);
CREATE INDEX IF NOT EXISTS idx_report_definitions_kind ON report_definitions(book_id, kind);

CREATE TABLE IF NOT EXISTS report_runs (
  id             INTEGER PRIMARY KEY,
  book_id        INTEGER NOT NULL,
  definition_id  INTEGER NOT NULL,
  params_hash    TEXT NOT NULL,
  as_of_seq      INTEGER NOT NULL,
  result_json    TEXT NOT NULL,
  created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(definition_id) REFERENCES report_definitions(id) ON DELETE CASCADE,
  UNIQUE(book_id, definition_id, params_hash, as_of_seq)
);

CREATE INDEX IF NOT EXISTS idx_report_runs_book_def ON report_runs(book_id, definition_id);
CREATE INDEX IF NOT EXISTS idx_report_runs_book_seq ON report_runs(book_id, as_of_seq);
