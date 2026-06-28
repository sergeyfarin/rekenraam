-- +goose Up

-- Online source connections (e.g. Trading 212 API credentials).
-- secret_ciphertext holds the API key sealed with AES-256-GCM via internal/secretbox;
-- the plaintext key is NEVER stored and NEVER returned by any API endpoint.
CREATE TABLE IF NOT EXISTS import_connections (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  source_kind TEXT NOT NULL CHECK (length(trim(source_kind)) > 0),
  display_name TEXT NOT NULL CHECK (length(trim(display_name)) > 0 AND display_name = trim(display_name)),
  secret_ciphertext TEXT NOT NULL,
  config_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(config_json)),
  fetch_cursor TEXT NOT NULL DEFAULT '',
  last_fetch_status TEXT NOT NULL DEFAULT '' CHECK (
    last_fetch_status IN ('', 'fetching', 'ready', 'failed')
  ),
  last_fetched_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (book_id, source_kind, display_name)
);

CREATE INDEX IF NOT EXISTS import_connections_book_idx
  ON import_connections (book_id, created_at DESC, id DESC);

-- Track which connection produced each batch so history is queryable.
ALTER TABLE import_batches ADD COLUMN connection_id INTEGER REFERENCES import_connections(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS import_batches_connection_idx
  ON import_batches (connection_id)
  WHERE connection_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS import_batches_connection_idx;
DROP INDEX IF EXISTS import_connections_book_idx;
-- SQLite does not support DROP COLUMN; the connection_id column is left on import_batches in the down path.
DROP TABLE IF EXISTS import_connections;
