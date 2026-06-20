-- +goose Up
CREATE TABLE market_data_ingest_runs_new (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  source_id INTEGER NOT NULL REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  trigger TEXT NOT NULL CHECK (trigger IN ('manual', 'scheduled', 'domain', 'recovery', 'provider_callback', 'import')),
  status TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed', 'partial')),
  started_at TEXT NOT NULL,
  finished_at TEXT,
  items_total INTEGER NOT NULL DEFAULT 0 CHECK (items_total >= 0),
  items_succeeded INTEGER NOT NULL DEFAULT 0 CHECK (items_succeeded >= 0),
  items_failed INTEGER NOT NULL DEFAULT 0 CHECK (items_failed >= 0),
  last_error TEXT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

INSERT INTO market_data_ingest_runs_new
SELECT * FROM market_data_ingest_runs;

CREATE TABLE market_data_ingest_items_backup AS
SELECT * FROM market_data_ingest_items;

DROP TABLE market_data_ingest_items;
DROP TABLE market_data_ingest_runs;
ALTER TABLE market_data_ingest_runs_new RENAME TO market_data_ingest_runs;

CREATE INDEX market_data_ingest_runs_book_idx
  ON market_data_ingest_runs (book_id, started_at DESC, id DESC);

CREATE TABLE market_data_ingest_items (
  id INTEGER PRIMARY KEY,
  run_id INTEGER NOT NULL REFERENCES market_data_ingest_runs(id) ON DELETE RESTRICT,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  item_kind TEXT NOT NULL CHECK (item_kind IN ('instrument', 'price', 'dividend', 'corporate_action')),
  status TEXT NOT NULL CHECK (status IN ('succeeded', 'failed', 'skipped')),
  provider_item_id TEXT,
  local_ref_table TEXT,
  local_ref_id INTEGER,
  error TEXT,
  raw_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

INSERT INTO market_data_ingest_items
SELECT * FROM market_data_ingest_items_backup;
DROP TABLE market_data_ingest_items_backup;

CREATE INDEX market_data_ingest_items_run_idx
  ON market_data_ingest_items (run_id, item_kind, status);

CREATE TABLE background_work_items (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  kind TEXT NOT NULL CHECK (length(trim(kind)) > 0 AND kind = trim(kind)),
  payload_version INTEGER NOT NULL DEFAULT 1 CHECK (payload_version > 0),
  payload_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(payload_json)),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed')),
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  available_at TEXT NOT NULL,
  lease_owner TEXT,
  lease_expires_at TEXT,
  last_error TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  completed_at TEXT,
  CHECK (
    (status = 'running' AND lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL)
    OR status != 'running'
  )
);

CREATE INDEX background_work_items_due_idx
  ON background_work_items (status, available_at, lease_expires_at, id);

-- +goose StatementBegin
CREATE TRIGGER fx_work_after_account_version_insert
AFTER INSERT ON account_versions
WHEN NEW.status = 'active'
  AND NEW.default_commodity_id IS NOT NULL
  AND EXISTS (
    SELECT 1 FROM commodities c
    WHERE c.id = NEW.default_commodity_id AND c.kind = 'currency'
  )
  AND NEW.default_commodity_id != COALESCE(
    (SELECT pp.base_commodity_id
     FROM accounts a JOIN pricing_policies pp ON pp.book_id = a.book_id
     WHERE a.id = NEW.account_id),
    (SELECT b.default_currency_commodity_id
     FROM accounts a JOIN books b ON b.id = a.book_id
     WHERE a.id = NEW.account_id),
    0
  )
  AND NOT EXISTS (
    SELECT 1 FROM account_versions prior
    WHERE prior.account_id = NEW.account_id AND prior.id != NEW.id
      AND prior.status = 'active'
      AND prior.default_commodity_id = NEW.default_commodity_id
  )
  AND NOT EXISTS (
    SELECT 1
    FROM current_account_versions other
    JOIN accounts other_account ON other_account.id = other.account_id
    JOIN accounts new_account ON new_account.id = NEW.account_id
    WHERE other_account.book_id = new_account.book_id
      AND other.account_id != NEW.account_id
      AND other.status = 'active'
      AND other.default_commodity_id = NEW.default_commodity_id
  )
BEGIN
  INSERT INTO background_work_items (
    book_id, kind, payload_json, available_at, created_at, updated_at
  )
  SELECT a.book_id, 'pricing.fx_coverage',
    json_object('reason', 'currency_activated', 'start_date', NEW.opened_on,
      'currency_id', NEW.default_commodity_id),
    NEW.recorded_at, NEW.recorded_at, NEW.recorded_at
  FROM accounts a WHERE a.id = NEW.account_id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER fx_work_after_posting_version_insert
AFTER INSERT ON posting_versions
WHEN EXISTS (
  SELECT 1 FROM transaction_versions tv
  WHERE tv.id = NEW.transaction_version_id AND tv.status IN ('draft', 'posted')
)
  AND EXISTS (SELECT 1 FROM commodities c WHERE c.id = NEW.commodity_id AND c.kind = 'currency')
  AND NEW.commodity_id != COALESCE(
    (SELECT base_commodity_id FROM pricing_policies WHERE book_id = NEW.book_id),
    (SELECT default_currency_commodity_id FROM books WHERE id = NEW.book_id),
    0
  )
BEGIN
  INSERT INTO background_work_items (
    book_id, kind, payload_json, available_at, created_at, updated_at
  ) VALUES (
    NEW.book_id, 'pricing.fx_coverage',
    json_object('reason', 'transaction_entered',
      'start_date', (SELECT entry_date FROM journal_entries WHERE id = NEW.journal_entry_id),
      'currency_id', NEW.commodity_id),
    strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
    strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
    strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
  );
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS fx_work_after_posting_version_insert;
DROP TRIGGER IF EXISTS fx_work_after_account_version_insert;
DROP INDEX IF EXISTS background_work_items_due_idx;
DROP TABLE IF EXISTS background_work_items;

CREATE TABLE market_data_ingest_runs_old (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  source_id INTEGER NOT NULL REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  trigger TEXT NOT NULL CHECK (trigger IN ('manual', 'scheduled', 'provider_callback', 'import')),
  status TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed', 'partial')),
  started_at TEXT NOT NULL,
  finished_at TEXT,
  items_total INTEGER NOT NULL DEFAULT 0 CHECK (items_total >= 0),
  items_succeeded INTEGER NOT NULL DEFAULT 0 CHECK (items_succeeded >= 0),
  items_failed INTEGER NOT NULL DEFAULT 0 CHECK (items_failed >= 0),
  last_error TEXT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

INSERT INTO market_data_ingest_runs_old
SELECT id, book_id, source_id,
  CASE WHEN trigger IN ('domain', 'recovery') THEN 'import' ELSE trigger END,
  status, started_at, finished_at, items_total, items_succeeded, items_failed,
  last_error, created_audit_event_id
FROM market_data_ingest_runs;

CREATE TABLE market_data_ingest_items_backup AS
SELECT mdi.* FROM market_data_ingest_items mdi
JOIN market_data_ingest_runs_old runs ON runs.id = mdi.run_id;

DROP TABLE market_data_ingest_items;
DROP TABLE market_data_ingest_runs;
ALTER TABLE market_data_ingest_runs_old RENAME TO market_data_ingest_runs;
CREATE INDEX market_data_ingest_runs_book_idx
  ON market_data_ingest_runs (book_id, started_at DESC, id DESC);

CREATE TABLE market_data_ingest_items (
  id INTEGER PRIMARY KEY,
  run_id INTEGER NOT NULL REFERENCES market_data_ingest_runs(id) ON DELETE RESTRICT,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  item_kind TEXT NOT NULL CHECK (item_kind IN ('instrument', 'price', 'dividend', 'corporate_action')),
  status TEXT NOT NULL CHECK (status IN ('succeeded', 'failed', 'skipped')),
  provider_item_id TEXT,
  local_ref_table TEXT,
  local_ref_id INTEGER,
  error TEXT,
  raw_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);
INSERT INTO market_data_ingest_items SELECT * FROM market_data_ingest_items_backup;
DROP TABLE market_data_ingest_items_backup;
CREATE INDEX market_data_ingest_items_run_idx
  ON market_data_ingest_items (run_id, item_kind, status);
