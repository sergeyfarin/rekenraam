-- +goose Up

CREATE TABLE IF NOT EXISTS import_profiles (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  name TEXT NOT NULL CHECK (length(trim(name)) > 0 AND name = trim(name)),
  adapter_kind TEXT NOT NULL CHECK (length(trim(adapter_kind)) > 0),
  config_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(config_json)),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS import_profiles_book_idx
  ON import_profiles (book_id);

CREATE TABLE IF NOT EXISTS import_batches (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  source_kind TEXT NOT NULL CHECK (length(trim(source_kind)) > 0),
  profile_id INTEGER REFERENCES import_profiles(id) ON DELETE SET NULL,
  status TEXT NOT NULL DEFAULT 'previewing' CHECK (
    status IN ('previewing', 'committed', 'partially_committed', 'discarded', 'failed')
  ),
  original_filename TEXT NOT NULL DEFAULT '',
  source_meta_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(source_meta_json)),
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS import_batches_book_created_idx
  ON import_batches (book_id, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS import_batch_events (
  id INTEGER PRIMARY KEY,
  batch_id INTEGER NOT NULL REFERENCES import_batches(id) ON DELETE CASCADE,
  event_kind TEXT NOT NULL CHECK (
    event_kind IN ('created', 'parsed', 'committed', 'partially_committed', 'discarded', 'failed', 'rolled_back')
  ),
  detail_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(detail_json)),
  audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS import_batch_events_batch_idx
  ON import_batch_events (batch_id, created_at ASC, id ASC);

CREATE TABLE IF NOT EXISTS import_staged_rows (
  id INTEGER PRIMARY KEY,
  batch_id INTEGER NOT NULL REFERENCES import_batches(id) ON DELETE CASCADE,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  row_index INTEGER NOT NULL CHECK (row_index >= 0),
  dedupe_fingerprint TEXT NOT NULL,
  raw_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(raw_json)),
  normalized_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(normalized_json)),
  dedupe_status TEXT NOT NULL DEFAULT 'new' CHECK (
    dedupe_status IN ('new', 'duplicate', 'needs_attention', 'excluded')
  ),
  resolution_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(resolution_json)),
  commit_status TEXT NOT NULL DEFAULT 'pending' CHECK (
    commit_status IN ('pending', 'committed', 'skipped', 'failed')
  ),
  committed_transaction_id INTEGER REFERENCES transactions(id) ON DELETE SET NULL,
  commit_error TEXT,
  UNIQUE (batch_id, row_index)
);

CREATE INDEX IF NOT EXISTS import_staged_rows_batch_idx
  ON import_staged_rows (batch_id, row_index ASC);

CREATE INDEX IF NOT EXISTS import_staged_rows_book_fingerprint_idx
  ON import_staged_rows (book_id, dedupe_fingerprint);

CREATE TABLE IF NOT EXISTS import_commit_identities (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  dedupe_fingerprint TEXT NOT NULL,
  committed_transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE RESTRICT,
  source_kind TEXT NOT NULL,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  created_at TEXT NOT NULL,
  UNIQUE (book_id, dedupe_fingerprint)
);

CREATE INDEX IF NOT EXISTS import_commit_identities_book_idx
  ON import_commit_identities (book_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS import_commit_identities;
DROP TABLE IF EXISTS import_staged_rows;
DROP TABLE IF EXISTS import_batch_events;
DROP TABLE IF EXISTS import_batches;
DROP TABLE IF EXISTS import_profiles;
