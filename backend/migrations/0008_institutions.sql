-- +goose Up
CREATE TABLE IF NOT EXISTS institutions (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_request_id TEXT
);

CREATE TABLE IF NOT EXISTS institution_versions (
  id INTEGER PRIMARY KEY,
  institution_id INTEGER NOT NULL REFERENCES institutions(id) ON DELETE RESTRICT,
  version_seq INTEGER NOT NULL,
  effective_from TEXT NOT NULL,
  recorded_at TEXT NOT NULL,
  changed_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  change_reason TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
  name TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('bank', 'credit_union', 'brokerage', 'card_issuer', 'lender', 'insurance', 'employer', 'rewards_program', 'government', 'other')),
  country_code TEXT,
  website TEXT,
  logo_url TEXT,
  logo_small_url TEXT,
  backdrop_url TEXT,
  address_json TEXT NOT NULL DEFAULT '{}',
  comment_markdown TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  UNIQUE (institution_id, version_seq)
);

CREATE INDEX IF NOT EXISTS institutions_book_idx
  ON institutions (book_id);

CREATE INDEX IF NOT EXISTS institution_versions_institution_seq_idx
  ON institution_versions (institution_id, version_seq DESC);

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS institution_versions_no_update
BEFORE UPDATE ON institution_versions
BEGIN
  SELECT RAISE(ABORT, 'institution_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS institution_versions_no_delete
BEFORE DELETE ON institution_versions
BEGIN
  SELECT RAISE(ABORT, 'institution_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS institution_versions_no_delete;
DROP TRIGGER IF EXISTS institution_versions_no_update;
DROP INDEX IF EXISTS institution_versions_institution_seq_idx;
DROP INDEX IF EXISTS institutions_book_idx;
DROP TABLE IF EXISTS institution_versions;
DROP TABLE IF EXISTS institutions;
