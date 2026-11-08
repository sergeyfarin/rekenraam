-- +goose Up
-- #480: the book's default currency is the one coverage base. Prices, the
-- attention sources, FX coverage, Net worth and Home all judge pairs against
-- it, so the pricing policy no longer carries a base of its own that could
-- diverge from what the reports value in.
--
-- SQLite cannot drop a column that carries a foreign key, so the table is
-- rebuilt. Nothing references pricing_policies; the two FX coverage triggers
-- read it and are recreated against books.default_currency_commodity_id.
DROP TRIGGER fx_work_after_posting_version_insert;
DROP TRIGGER fx_work_after_account_version_insert;

CREATE TABLE pricing_policies_next (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT UNIQUE,
  default_source_id INTEGER REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  refresh_enabled INTEGER NOT NULL DEFAULT 1 CHECK (refresh_enabled IN (0, 1)),
  refresh_hour_utc INTEGER NOT NULL DEFAULT 4 CHECK (refresh_hour_utc BETWEEN 0 AND 23),
  refresh_minute_utc INTEGER NOT NULL DEFAULT 0 CHECK (refresh_minute_utc BETWEEN 0 AND 59),
  refresh_hour_local INTEGER NOT NULL DEFAULT 4 CHECK (refresh_hour_local BETWEEN 0 AND 23),
  refresh_minute_local INTEGER NOT NULL DEFAULT 0 CHECK (refresh_minute_local BETWEEN 0 AND 59),
  max_backfill_days INTEGER NOT NULL DEFAULT 370 CHECK (max_backfill_days >= 1),
  staleness_max_days INTEGER NOT NULL DEFAULT 3 CHECK (staleness_max_days >= 1),
  triangulation_max_hops INTEGER NOT NULL DEFAULT 1 CHECK (triangulation_max_hops BETWEEN 0 AND 4),
  rounding_mode TEXT NOT NULL DEFAULT 'half_up' CHECK (rounding_mode IN ('half_up', 'half_even', 'down', 'up')),
  prefer_official_fx INTEGER NOT NULL DEFAULT 1 CHECK (prefer_official_fx IN (0, 1)),
  weekend_policy TEXT NOT NULL DEFAULT 'skip' CHECK (weekend_policy IN ('skip', 'fill_previous', 'download')),
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

INSERT INTO pricing_policies_next (
  id, book_id, default_source_id, refresh_enabled, refresh_hour_utc, refresh_minute_utc,
  refresh_hour_local, refresh_minute_local, max_backfill_days, staleness_max_days,
  triangulation_max_hops, rounding_mode, prefer_official_fx, weekend_policy, created_at,
  created_by_user_id, updated_at, updated_by_user_id, created_audit_event_id, updated_audit_event_id
)
SELECT
  id, book_id, default_source_id, refresh_enabled, refresh_hour_utc, refresh_minute_utc,
  refresh_hour_local, refresh_minute_local, max_backfill_days, staleness_max_days,
  triangulation_max_hops, rounding_mode, prefer_official_fx, weekend_policy, created_at,
  created_by_user_id, updated_at, updated_by_user_id, created_audit_event_id, updated_audit_event_id
FROM pricing_policies;

DROP TABLE pricing_policies;
ALTER TABLE pricing_policies_next RENAME TO pricing_policies;

-- +goose StatementBegin
CREATE TRIGGER fx_work_after_posting_version_insert
AFTER INSERT ON posting_versions
WHEN EXISTS (SELECT 1 FROM transaction_versions tv WHERE tv.id = NEW.transaction_version_id AND tv.status = 'posted')
  AND EXISTS (SELECT 1 FROM commodities c WHERE c.id = NEW.commodity_id AND c.kind = 'currency')
  AND NEW.commodity_id != COALESCE(
    (SELECT default_currency_commodity_id FROM books WHERE id = NEW.book_id),
    0
  )
  AND NOT EXISTS (
    SELECT 1 FROM background_work_items bw
    WHERE bw.book_id = NEW.book_id AND bw.kind = 'pricing.fx_coverage'
      AND bw.status = 'pending' AND bw.attempts = 0
      AND json_extract(bw.payload_json, '$.next') IS NULL
      AND json_extract(bw.payload_json, '$.retry') IS NULL
      AND json_extract(bw.payload_json, '$.currency_id') = NEW.commodity_id
      AND json_extract(bw.payload_json, '$.start_date') <= (SELECT entry_date FROM journal_entries WHERE id = NEW.journal_entry_id)
  )
BEGIN
  INSERT INTO background_work_items (book_id, kind, payload_json, available_at, created_at, updated_at)
  VALUES (
    NEW.book_id, 'pricing.fx_coverage',
    json_object('reason', 'transaction_entered', 'start_date', (SELECT entry_date FROM journal_entries WHERE id = NEW.journal_entry_id), 'currency_id', NEW.commodity_id),
    strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
  )
  ON CONFLICT(book_id, kind, payload_json) WHERE status IN ('pending', 'running') DO NOTHING;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER fx_work_after_account_version_insert
AFTER INSERT ON account_versions
WHEN NEW.status = 'active'
  AND EXISTS (SELECT 1 FROM commodities c WHERE c.id = NEW.default_commodity_id AND c.kind = 'currency')
  AND NEW.default_commodity_id != COALESCE(
    (SELECT b.default_currency_commodity_id FROM accounts a JOIN books b ON b.id = a.book_id WHERE a.id = NEW.account_id),
    0
  )
  AND NOT EXISTS (
    SELECT 1 FROM account_versions prior
    WHERE prior.account_id = NEW.account_id AND prior.id != NEW.id
      AND prior.status = 'active' AND prior.default_commodity_id = NEW.default_commodity_id
  )
  AND NOT EXISTS (
    SELECT 1 FROM account_versions other
    JOIN accounts other_account ON other_account.id = other.account_id
    JOIN accounts new_account ON new_account.id = NEW.account_id
    WHERE other_account.book_id = new_account.book_id
      AND other.id = (SELECT candidate.id FROM account_versions candidate
        WHERE candidate.account_id = other.account_id
          AND candidate.effective_from <= NEW.effective_from
        ORDER BY candidate.effective_from DESC, candidate.version_seq DESC LIMIT 1)
      AND other.account_id != NEW.account_id
      AND other.status = 'active' AND other.default_commodity_id = NEW.default_commodity_id
  )
BEGIN
  INSERT INTO background_work_items (book_id, kind, payload_json, available_at, created_at, updated_at)
  SELECT a.book_id, 'pricing.fx_coverage',
    json_object('reason', 'currency_activated', 'start_date', NEW.opened_on, 'currency_id', NEW.default_commodity_id),
    NEW.recorded_at, NEW.recorded_at, NEW.recorded_at
  FROM accounts a WHERE a.id = NEW.account_id
  ON CONFLICT(book_id, kind, payload_json) WHERE status IN ('pending', 'running') DO NOTHING;
END;
-- +goose StatementEnd


-- +goose Down
DROP TRIGGER fx_work_after_posting_version_insert;
DROP TRIGGER fx_work_after_account_version_insert;

CREATE TABLE pricing_policies_previous (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT UNIQUE,
  base_commodity_id INTEGER REFERENCES commodities(id) ON DELETE RESTRICT,
  default_source_id INTEGER REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  refresh_enabled INTEGER NOT NULL DEFAULT 1 CHECK (refresh_enabled IN (0, 1)),
  refresh_hour_utc INTEGER NOT NULL DEFAULT 4 CHECK (refresh_hour_utc BETWEEN 0 AND 23),
  refresh_minute_utc INTEGER NOT NULL DEFAULT 0 CHECK (refresh_minute_utc BETWEEN 0 AND 59),
  refresh_hour_local INTEGER NOT NULL DEFAULT 4 CHECK (refresh_hour_local BETWEEN 0 AND 23),
  refresh_minute_local INTEGER NOT NULL DEFAULT 0 CHECK (refresh_minute_local BETWEEN 0 AND 59),
  max_backfill_days INTEGER NOT NULL DEFAULT 370 CHECK (max_backfill_days >= 1),
  staleness_max_days INTEGER NOT NULL DEFAULT 3 CHECK (staleness_max_days >= 1),
  triangulation_max_hops INTEGER NOT NULL DEFAULT 1 CHECK (triangulation_max_hops BETWEEN 0 AND 4),
  rounding_mode TEXT NOT NULL DEFAULT 'half_up' CHECK (rounding_mode IN ('half_up', 'half_even', 'down', 'up')),
  prefer_official_fx INTEGER NOT NULL DEFAULT 1 CHECK (prefer_official_fx IN (0, 1)),
  weekend_policy TEXT NOT NULL DEFAULT 'skip' CHECK (weekend_policy IN ('skip', 'fill_previous', 'download')),
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

INSERT INTO pricing_policies_previous (
  id, book_id, default_source_id, refresh_enabled, refresh_hour_utc, refresh_minute_utc,
  refresh_hour_local, refresh_minute_local, max_backfill_days, staleness_max_days,
  triangulation_max_hops, rounding_mode, prefer_official_fx, weekend_policy, created_at,
  created_by_user_id, updated_at, updated_by_user_id, created_audit_event_id, updated_audit_event_id
)
SELECT
  id, book_id, default_source_id, refresh_enabled, refresh_hour_utc, refresh_minute_utc,
  refresh_hour_local, refresh_minute_local, max_backfill_days, staleness_max_days,
  triangulation_max_hops, rounding_mode, prefer_official_fx, weekend_policy, created_at,
  created_by_user_id, updated_at, updated_by_user_id, created_audit_event_id, updated_audit_event_id
FROM pricing_policies;

DROP TABLE pricing_policies;
ALTER TABLE pricing_policies_previous RENAME TO pricing_policies;

-- +goose StatementBegin
CREATE TRIGGER fx_work_after_posting_version_insert
AFTER INSERT ON posting_versions
WHEN EXISTS (SELECT 1 FROM transaction_versions tv WHERE tv.id = NEW.transaction_version_id AND tv.status = 'posted')
  AND EXISTS (SELECT 1 FROM commodities c WHERE c.id = NEW.commodity_id AND c.kind = 'currency')
  AND NEW.commodity_id != COALESCE(
    (SELECT base_commodity_id FROM pricing_policies WHERE book_id = NEW.book_id),
    (SELECT default_currency_commodity_id FROM books WHERE id = NEW.book_id),
    0
  )
  AND NOT EXISTS (
    SELECT 1 FROM background_work_items bw
    WHERE bw.book_id = NEW.book_id AND bw.kind = 'pricing.fx_coverage'
      AND bw.status = 'pending' AND bw.attempts = 0
      AND json_extract(bw.payload_json, '$.next') IS NULL
      AND json_extract(bw.payload_json, '$.retry') IS NULL
      AND json_extract(bw.payload_json, '$.currency_id') = NEW.commodity_id
      AND json_extract(bw.payload_json, '$.start_date') <= (SELECT entry_date FROM journal_entries WHERE id = NEW.journal_entry_id)
  )
BEGIN
  INSERT INTO background_work_items (book_id, kind, payload_json, available_at, created_at, updated_at)
  VALUES (
    NEW.book_id, 'pricing.fx_coverage',
    json_object('reason', 'transaction_entered', 'start_date', (SELECT entry_date FROM journal_entries WHERE id = NEW.journal_entry_id), 'currency_id', NEW.commodity_id),
    strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
  )
  ON CONFLICT(book_id, kind, payload_json) WHERE status IN ('pending', 'running') DO NOTHING;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER fx_work_after_account_version_insert
AFTER INSERT ON account_versions
WHEN NEW.status = 'active'
  AND EXISTS (SELECT 1 FROM commodities c WHERE c.id = NEW.default_commodity_id AND c.kind = 'currency')
  AND NEW.default_commodity_id != COALESCE(
    (SELECT pp.base_commodity_id FROM accounts a JOIN pricing_policies pp ON pp.book_id = a.book_id WHERE a.id = NEW.account_id),
    (SELECT b.default_currency_commodity_id FROM accounts a JOIN books b ON b.id = a.book_id WHERE a.id = NEW.account_id),
    0
  )
  AND NOT EXISTS (
    SELECT 1 FROM account_versions prior
    WHERE prior.account_id = NEW.account_id AND prior.id != NEW.id
      AND prior.status = 'active' AND prior.default_commodity_id = NEW.default_commodity_id
  )
  AND NOT EXISTS (
    SELECT 1 FROM account_versions other
    JOIN accounts other_account ON other_account.id = other.account_id
    JOIN accounts new_account ON new_account.id = NEW.account_id
    WHERE other_account.book_id = new_account.book_id
      AND other.id = (SELECT candidate.id FROM account_versions candidate
        WHERE candidate.account_id = other.account_id
          AND candidate.effective_from <= NEW.effective_from
        ORDER BY candidate.effective_from DESC, candidate.version_seq DESC LIMIT 1)
      AND other.account_id != NEW.account_id
      AND other.status = 'active' AND other.default_commodity_id = NEW.default_commodity_id
  )
BEGIN
  INSERT INTO background_work_items (book_id, kind, payload_json, available_at, created_at, updated_at)
  SELECT a.book_id, 'pricing.fx_coverage',
    json_object('reason', 'currency_activated', 'start_date', NEW.opened_on, 'currency_id', NEW.default_commodity_id),
    NEW.recorded_at, NEW.recorded_at, NEW.recorded_at
  FROM accounts a WHERE a.id = NEW.account_id
  ON CONFLICT(book_id, kind, payload_json) WHERE status IN ('pending', 'running') DO NOTHING;
END;
-- +goose StatementEnd
