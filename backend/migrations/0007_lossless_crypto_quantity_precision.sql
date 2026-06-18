-- +goose Up
-- Quantity coefficients move to canonical decimal text so SQLite never coerces
-- values above int64 to REAL. Scale 24 is a technical ceiling; commodity-kind
-- policy keeps non-crypto commodities at 12.

DROP TRIGGER IF EXISTS posting_versions_account_version_valid;
DROP TRIGGER IF EXISTS posting_versions_commodity_scale_valid;
DROP TRIGGER IF EXISTS investment_lots_values_valid_insert;
DROP TRIGGER IF EXISTS investment_lots_values_valid_update;
DROP TRIGGER IF EXISTS commodity_versions_no_update;
DROP TRIGGER IF EXISTS account_versions_no_update;
DROP TRIGGER IF EXISTS posting_versions_no_update;

ALTER TABLE commodity_versions RENAME COLUMN standard_scale TO standard_scale_v1;
ALTER TABLE commodity_versions RENAME COLUMN max_quantity_scale TO max_quantity_scale_v1;
ALTER TABLE commodity_versions ADD COLUMN standard_scale INTEGER NOT NULL DEFAULT 0 CHECK (standard_scale BETWEEN 0 AND 24);
ALTER TABLE commodity_versions ADD COLUMN max_quantity_scale INTEGER NOT NULL DEFAULT 0 CHECK (max_quantity_scale BETWEEN 0 AND 24 AND max_quantity_scale >= standard_scale);
UPDATE commodity_versions SET standard_scale = standard_scale_v1, max_quantity_scale = max_quantity_scale_v1;
ALTER TABLE commodity_versions DROP COLUMN max_quantity_scale_v1;
ALTER TABLE commodity_versions DROP COLUMN standard_scale_v1;

ALTER TABLE account_versions RENAME COLUMN quantity_scale_override TO quantity_scale_override_v1;
ALTER TABLE account_versions ADD COLUMN quantity_scale_override INTEGER CHECK (quantity_scale_override IS NULL OR quantity_scale_override BETWEEN 0 AND 24);
UPDATE account_versions SET quantity_scale_override = quantity_scale_override_v1;
ALTER TABLE account_versions DROP COLUMN quantity_scale_override_v1;

ALTER TABLE posting_versions RENAME COLUMN quantity_value TO quantity_value_v1;
ALTER TABLE posting_versions RENAME COLUMN quantity_scale TO quantity_scale_v1;
ALTER TABLE posting_versions ADD COLUMN quantity_value TEXT NOT NULL DEFAULT '0' CHECK (length(quantity_value) BETWEEN 1 AND 39);
ALTER TABLE posting_versions ADD COLUMN quantity_scale INTEGER NOT NULL DEFAULT 0 CHECK (quantity_scale BETWEEN 0 AND 24);
UPDATE posting_versions SET quantity_value = CAST(quantity_value_v1 AS TEXT), quantity_scale = quantity_scale_v1;
ALTER TABLE posting_versions DROP COLUMN quantity_value_v1;
ALTER TABLE posting_versions DROP COLUMN quantity_scale_v1;

ALTER TABLE reconciliation_sessions RENAME COLUMN statement_balance_value TO statement_balance_value_v1;
ALTER TABLE reconciliation_sessions RENAME COLUMN statement_balance_scale TO statement_balance_scale_v1;
ALTER TABLE reconciliation_sessions RENAME COLUMN starting_balance_value TO starting_balance_value_v1;
ALTER TABLE reconciliation_sessions RENAME COLUMN starting_balance_scale TO starting_balance_scale_v1;
ALTER TABLE reconciliation_sessions RENAME COLUMN selected_balance_value TO selected_balance_value_v1;
ALTER TABLE reconciliation_sessions RENAME COLUMN selected_balance_scale TO selected_balance_scale_v1;
ALTER TABLE reconciliation_sessions RENAME COLUMN difference_value TO difference_value_v1;
ALTER TABLE reconciliation_sessions RENAME COLUMN difference_scale TO difference_scale_v1;
ALTER TABLE reconciliation_sessions ADD COLUMN statement_balance_value TEXT NOT NULL DEFAULT '0' CHECK (length(statement_balance_value) BETWEEN 1 AND 39);
ALTER TABLE reconciliation_sessions ADD COLUMN statement_balance_scale INTEGER NOT NULL DEFAULT 0 CHECK (statement_balance_scale BETWEEN 0 AND 24);
ALTER TABLE reconciliation_sessions ADD COLUMN starting_balance_value TEXT NOT NULL DEFAULT '0' CHECK (length(starting_balance_value) BETWEEN 1 AND 39);
ALTER TABLE reconciliation_sessions ADD COLUMN starting_balance_scale INTEGER NOT NULL DEFAULT 0 CHECK (starting_balance_scale BETWEEN 0 AND 24);
ALTER TABLE reconciliation_sessions ADD COLUMN selected_balance_value TEXT NOT NULL DEFAULT '0' CHECK (length(selected_balance_value) BETWEEN 1 AND 39);
ALTER TABLE reconciliation_sessions ADD COLUMN selected_balance_scale INTEGER NOT NULL DEFAULT 0 CHECK (selected_balance_scale BETWEEN 0 AND 24);
ALTER TABLE reconciliation_sessions ADD COLUMN difference_value TEXT NOT NULL DEFAULT '0' CHECK (length(difference_value) BETWEEN 1 AND 39);
ALTER TABLE reconciliation_sessions ADD COLUMN difference_scale INTEGER NOT NULL DEFAULT 0 CHECK (difference_scale BETWEEN 0 AND 24);
UPDATE reconciliation_sessions SET
  statement_balance_value = CAST(statement_balance_value_v1 AS TEXT), statement_balance_scale = statement_balance_scale_v1,
  starting_balance_value = CAST(starting_balance_value_v1 AS TEXT), starting_balance_scale = starting_balance_scale_v1,
  selected_balance_value = CAST(selected_balance_value_v1 AS TEXT), selected_balance_scale = selected_balance_scale_v1,
  difference_value = CAST(difference_value_v1 AS TEXT), difference_scale = difference_scale_v1;
ALTER TABLE reconciliation_sessions DROP COLUMN statement_balance_value_v1;
ALTER TABLE reconciliation_sessions DROP COLUMN statement_balance_scale_v1;
ALTER TABLE reconciliation_sessions DROP COLUMN starting_balance_value_v1;
ALTER TABLE reconciliation_sessions DROP COLUMN starting_balance_scale_v1;
ALTER TABLE reconciliation_sessions DROP COLUMN selected_balance_value_v1;
ALTER TABLE reconciliation_sessions DROP COLUMN selected_balance_scale_v1;
ALTER TABLE reconciliation_sessions DROP COLUMN difference_value_v1;
ALTER TABLE reconciliation_sessions DROP COLUMN difference_scale_v1;

ALTER TABLE reconciliation_session_postings RENAME COLUMN quantity_value TO quantity_value_v1;
ALTER TABLE reconciliation_session_postings RENAME COLUMN quantity_scale TO quantity_scale_v1;
ALTER TABLE reconciliation_session_postings ADD COLUMN quantity_value TEXT NOT NULL DEFAULT '0' CHECK (length(quantity_value) BETWEEN 1 AND 39);
ALTER TABLE reconciliation_session_postings ADD COLUMN quantity_scale INTEGER NOT NULL DEFAULT 0 CHECK (quantity_scale BETWEEN 0 AND 24);
UPDATE reconciliation_session_postings SET quantity_value = CAST(quantity_value_v1 AS TEXT), quantity_scale = quantity_scale_v1;
ALTER TABLE reconciliation_session_postings DROP COLUMN quantity_value_v1;
ALTER TABLE reconciliation_session_postings DROP COLUMN quantity_scale_v1;

ALTER TABLE reconciliation_checkpoints RENAME COLUMN statement_balance_value TO statement_balance_value_v1;
ALTER TABLE reconciliation_checkpoints RENAME COLUMN statement_balance_scale TO statement_balance_scale_v1;
ALTER TABLE reconciliation_checkpoints ADD COLUMN statement_balance_value TEXT NOT NULL DEFAULT '0' CHECK (length(statement_balance_value) BETWEEN 1 AND 39);
ALTER TABLE reconciliation_checkpoints ADD COLUMN statement_balance_scale INTEGER NOT NULL DEFAULT 0 CHECK (statement_balance_scale BETWEEN 0 AND 24);
UPDATE reconciliation_checkpoints SET statement_balance_value = CAST(statement_balance_value_v1 AS TEXT), statement_balance_scale = statement_balance_scale_v1;
ALTER TABLE reconciliation_checkpoints DROP COLUMN statement_balance_value_v1;
ALTER TABLE reconciliation_checkpoints DROP COLUMN statement_balance_scale_v1;

ALTER TABLE reconciliation_checkpoint_postings RENAME COLUMN quantity_value TO quantity_value_v1;
ALTER TABLE reconciliation_checkpoint_postings RENAME COLUMN quantity_scale TO quantity_scale_v1;
ALTER TABLE reconciliation_checkpoint_postings ADD COLUMN quantity_value TEXT NOT NULL DEFAULT '0' CHECK (length(quantity_value) BETWEEN 1 AND 39);
ALTER TABLE reconciliation_checkpoint_postings ADD COLUMN quantity_scale INTEGER NOT NULL DEFAULT 0 CHECK (quantity_scale BETWEEN 0 AND 24);
UPDATE reconciliation_checkpoint_postings SET quantity_value = CAST(quantity_value_v1 AS TEXT), quantity_scale = quantity_scale_v1;
ALTER TABLE reconciliation_checkpoint_postings DROP COLUMN quantity_value_v1;
ALTER TABLE reconciliation_checkpoint_postings DROP COLUMN quantity_scale_v1;

ALTER TABLE investment_lots RENAME COLUMN quantity_value TO quantity_value_v1;
ALTER TABLE investment_lots RENAME COLUMN quantity_scale TO quantity_scale_v1;
ALTER TABLE investment_lots RENAME COLUMN remaining_quantity_value TO remaining_quantity_value_v1;
ALTER TABLE investment_lots RENAME COLUMN remaining_quantity_scale TO remaining_quantity_scale_v1;
ALTER TABLE investment_lots ADD COLUMN quantity_value TEXT NOT NULL DEFAULT '0' CHECK (length(quantity_value) BETWEEN 1 AND 38);
ALTER TABLE investment_lots ADD COLUMN quantity_scale INTEGER NOT NULL DEFAULT 0 CHECK (quantity_scale BETWEEN 0 AND 24);
ALTER TABLE investment_lots ADD COLUMN remaining_quantity_value TEXT NOT NULL DEFAULT '0' CHECK (length(remaining_quantity_value) BETWEEN 1 AND 38);
ALTER TABLE investment_lots ADD COLUMN remaining_quantity_scale INTEGER NOT NULL DEFAULT 0 CHECK (remaining_quantity_scale BETWEEN 0 AND 24);
UPDATE investment_lots SET
  quantity_value = CAST(quantity_value_v1 AS TEXT), quantity_scale = quantity_scale_v1,
  remaining_quantity_value = CAST(remaining_quantity_value_v1 AS TEXT), remaining_quantity_scale = remaining_quantity_scale_v1;
ALTER TABLE investment_lots DROP COLUMN quantity_value_v1;
ALTER TABLE investment_lots DROP COLUMN quantity_scale_v1;
ALTER TABLE investment_lots DROP COLUMN remaining_quantity_value_v1;
ALTER TABLE investment_lots DROP COLUMN remaining_quantity_scale_v1;

ALTER TABLE investment_lot_events RENAME COLUMN quantity_value TO quantity_value_v1;
ALTER TABLE investment_lot_events RENAME COLUMN quantity_scale TO quantity_scale_v1;
ALTER TABLE investment_lot_events ADD COLUMN quantity_value TEXT NOT NULL DEFAULT '0' CHECK (length(quantity_value) BETWEEN 1 AND 39);
ALTER TABLE investment_lot_events ADD COLUMN quantity_scale INTEGER NOT NULL DEFAULT 0 CHECK (quantity_scale BETWEEN 0 AND 24);
UPDATE investment_lot_events SET quantity_value = CAST(quantity_value_v1 AS TEXT), quantity_scale = quantity_scale_v1;
ALTER TABLE investment_lot_events DROP COLUMN quantity_value_v1;
ALTER TABLE investment_lot_events DROP COLUMN quantity_scale_v1;

-- +goose StatementBegin
CREATE TRIGGER commodity_versions_kind_scale_valid
BEFORE INSERT ON commodity_versions
WHEN NEW.standard_scale > 24
  OR NEW.max_quantity_scale > 24
  OR (
    (SELECT kind FROM commodities WHERE id = NEW.commodity_id) <> 'crypto'
    AND (NEW.standard_scale > 12 OR NEW.max_quantity_scale > 12)
  )
BEGIN
  SELECT RAISE(ABORT, 'commodity precision is invalid for commodity kind');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER commodity_versions_no_update
BEFORE UPDATE ON commodity_versions
BEGIN
  SELECT RAISE(ABORT, 'commodity_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER account_versions_no_update
BEFORE UPDATE ON account_versions
BEGIN
  SELECT RAISE(ABORT, 'account_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER posting_versions_no_update
BEFORE UPDATE ON posting_versions
BEGIN
  SELECT RAISE(ABORT, 'posting_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER posting_versions_account_version_valid
BEFORE INSERT ON posting_versions
WHEN NOT EXISTS (
  SELECT 1
  FROM journal_entries je
  JOIN account_versions av ON av.account_id = NEW.account_id
  WHERE je.id = NEW.journal_entry_id
    AND av.id = (
      SELECT asof_av.id FROM account_versions asof_av
      WHERE asof_av.account_id = NEW.account_id AND asof_av.effective_from <= je.entry_date
      ORDER BY asof_av.effective_from DESC, asof_av.version_seq DESC LIMIT 1
    )
    AND av.status = 'active' AND av.allows_postings = 1
    AND (av.opened_on IS NULL OR je.entry_date >= av.opened_on)
    AND (av.closed_on IS NULL OR je.entry_date <= av.closed_on)
    AND (av.default_commodity_id IS NULL OR av.default_commodity_id = NEW.commodity_id)
    AND (av.quantity_scale_override IS NULL OR NEW.quantity_scale <= av.quantity_scale_override)
)
BEGIN
  SELECT RAISE(ABORT, 'posting account is not eligible on entry date');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER posting_versions_commodity_scale_valid
BEFORE INSERT ON posting_versions
WHEN NOT EXISTS (
  SELECT 1
  FROM journal_entries je
  JOIN commodity_versions cv ON cv.commodity_id = NEW.commodity_id
  JOIN commodities c ON c.id = cv.commodity_id
  WHERE je.id = NEW.journal_entry_id
    AND cv.id = (
      SELECT asof_cv.id FROM commodity_versions asof_cv
      WHERE asof_cv.commodity_id = NEW.commodity_id AND asof_cv.effective_from <= je.entry_date
      ORDER BY asof_cv.effective_from DESC, asof_cv.version_seq DESC LIMIT 1
    )
    AND cv.status = 'active'
    AND NEW.quantity_scale <= cv.max_quantity_scale
    AND NEW.quantity_scale <= CASE WHEN c.kind = 'crypto' THEN 24 ELSE 12 END
)
BEGIN
  SELECT RAISE(ABORT, 'posting commodity scale is invalid');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER investment_lots_values_valid_insert
BEFORE INSERT ON investment_lots
WHEN NEW.quantity_value = '0' OR substr(NEW.quantity_value, 1, 1) = '-'
  OR substr(NEW.remaining_quantity_value, 1, 1) = '-'
  OR NEW.cost_basis_value < 0 OR NEW.remaining_cost_basis_value < 0
BEGIN
  SELECT RAISE(ABORT, 'investment lot values are invalid');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER investment_lots_values_valid_update
BEFORE UPDATE ON investment_lots
WHEN NEW.quantity_value = '0' OR substr(NEW.quantity_value, 1, 1) = '-'
  OR substr(NEW.remaining_quantity_value, 1, 1) = '-'
  OR NEW.cost_basis_value < 0 OR NEW.remaining_cost_basis_value < 0
BEGIN
  SELECT RAISE(ABORT, 'investment lot values are invalid');
END;
-- +goose StatementEnd
