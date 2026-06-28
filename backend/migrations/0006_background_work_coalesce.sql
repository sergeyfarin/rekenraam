-- +goose Up
UPDATE background_work_items
SET status = 'completed',
    lease_owner = NULL,
    lease_expires_at = NULL,
    completed_at = COALESCE(completed_at, strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE status IN ('pending', 'running')
  AND id NOT IN (
    SELECT MIN(id)
    FROM background_work_items
    WHERE status IN ('pending', 'running')
    GROUP BY book_id, kind, payload_json
  );

CREATE UNIQUE INDEX IF NOT EXISTS background_work_items_active_unique_idx
  ON background_work_items (book_id, kind, payload_json)
  WHERE status IN ('pending', 'running');

DROP TRIGGER IF EXISTS fx_work_after_account_version_insert;
DROP TRIGGER IF EXISTS fx_work_after_posting_version_insert;

-- +goose StatementBegin
CREATE TRIGGER fx_work_after_account_version_insert
AFTER INSERT ON account_versions
WHEN NEW.status = 'active'
  AND EXISTS (SELECT 1 FROM commodities c WHERE c.id = NEW.default_commodity_id AND c.kind = 'currency')
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
  FROM accounts a WHERE a.id = NEW.account_id
  ON CONFLICT(book_id, kind, payload_json) WHERE status IN ('pending', 'running') DO NOTHING;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER fx_work_after_posting_version_insert
AFTER INSERT ON posting_versions
WHEN EXISTS (
  SELECT 1 FROM transaction_versions tv
  WHERE tv.id = NEW.transaction_version_id AND tv.status = 'posted'
)
  AND EXISTS (SELECT 1 FROM commodities c WHERE c.id = NEW.commodity_id AND c.kind = 'currency')
  AND NEW.commodity_id != COALESCE(
    (SELECT base_commodity_id FROM pricing_policies WHERE book_id = NEW.book_id),
    (SELECT default_currency_commodity_id FROM books WHERE id = NEW.book_id),
    0
  )
  AND NOT EXISTS (
    SELECT 1 FROM background_work_items bw
    WHERE bw.book_id = NEW.book_id
      AND bw.kind = 'pricing.fx_coverage'
      AND bw.status IN ('pending', 'running')
      AND json_extract(bw.payload_json, '$.currency_id') = NEW.commodity_id
      AND json_extract(bw.payload_json, '$.start_date') <= (SELECT entry_date FROM journal_entries WHERE id = NEW.journal_entry_id)
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
  )
  ON CONFLICT(book_id, kind, payload_json) WHERE status IN ('pending', 'running') DO NOTHING;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS fx_work_after_posting_version_insert;
DROP TRIGGER IF EXISTS fx_work_after_account_version_insert;
DROP INDEX IF EXISTS background_work_items_active_unique_idx;

-- +goose StatementBegin
CREATE TRIGGER fx_work_after_account_version_insert
AFTER INSERT ON account_versions
WHEN NEW.status = 'active'
  AND EXISTS (SELECT 1 FROM commodities c WHERE c.id = NEW.default_commodity_id AND c.kind = 'currency')
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
  WHERE tv.id = NEW.transaction_version_id AND tv.status = 'posted'
)
  AND EXISTS (SELECT 1 FROM commodities c WHERE c.id = NEW.commodity_id AND c.kind = 'currency')
  AND NEW.commodity_id != COALESCE(
    (SELECT base_commodity_id FROM pricing_policies WHERE book_id = NEW.book_id),
    (SELECT default_currency_commodity_id FROM books WHERE id = NEW.book_id),
    0
  )
  AND NOT EXISTS (
    SELECT 1 FROM background_work_items bw
    WHERE bw.book_id = NEW.book_id
      AND bw.kind = 'pricing.fx_coverage'
      AND bw.status IN ('pending', 'running')
      AND json_extract(bw.payload_json, '$.currency_id') = NEW.commodity_id
      AND json_extract(bw.payload_json, '$.start_date') <= (SELECT entry_date FROM journal_entries WHERE id = NEW.journal_entry_id)
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
