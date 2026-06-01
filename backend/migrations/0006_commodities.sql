-- +goose Up
ALTER TABLE books RENAME COLUMN base_currency_commodity_id TO default_currency_commodity_id;

CREATE TABLE IF NOT EXISTS commodities (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  code TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('currency', 'security', 'crypto', 'reward', 'commodity')),
  is_builtin INTEGER NOT NULL CHECK (is_builtin IN (0, 1)),
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_request_id TEXT,
  UNIQUE (book_id, code)
);

CREATE TABLE IF NOT EXISTS commodity_versions (
  id INTEGER PRIMARY KEY,
  commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  version_seq INTEGER NOT NULL CHECK (version_seq > 0),
  effective_from TEXT NOT NULL,
  recorded_at TEXT NOT NULL,
  changed_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  change_reason TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
  symbol TEXT NOT NULL,
  display_symbol TEXT NOT NULL,
  name TEXT NOT NULL,
  standard_scale INTEGER NOT NULL CHECK (standard_scale BETWEEN 0 AND 12),
  max_quantity_scale INTEGER NOT NULL CHECK (max_quantity_scale BETWEEN 0 AND 12 AND max_quantity_scale >= standard_scale),
  metadata_json TEXT NOT NULL DEFAULT '{}',
  UNIQUE (commodity_id, version_seq)
);

CREATE INDEX IF NOT EXISTS commodities_book_kind_idx
  ON commodities (book_id, kind, code);

CREATE INDEX IF NOT EXISTS commodity_versions_commodity_seq_idx
  ON commodity_versions (commodity_id, version_seq DESC);

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS commodity_versions_no_update
BEFORE UPDATE ON commodity_versions
BEGIN
  SELECT RAISE(ABORT, 'commodity_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS commodity_versions_no_delete
BEFORE DELETE ON commodity_versions
BEGIN
  SELECT RAISE(ABORT, 'commodity_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS books_default_currency_must_exist
BEFORE UPDATE OF default_currency_commodity_id ON books
WHEN NEW.default_currency_commodity_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM commodities
    WHERE id = NEW.default_currency_commodity_id
      AND book_id = NEW.id
      AND kind = 'currency'
  )
BEGIN
  SELECT RAISE(ABORT, 'book default currency must reference a currency in the same book');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS books_default_currency_must_exist;
DROP TRIGGER IF EXISTS commodity_versions_no_delete;
DROP TRIGGER IF EXISTS commodity_versions_no_update;
DROP INDEX IF EXISTS commodity_versions_commodity_seq_idx;
DROP INDEX IF EXISTS commodities_book_kind_idx;
DROP TABLE IF EXISTS commodity_versions;
DROP TABLE IF EXISTS commodities;
ALTER TABLE books RENAME COLUMN default_currency_commodity_id TO base_currency_commodity_id;
