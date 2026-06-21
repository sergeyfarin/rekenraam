-- +goose Up

-- Add same-day ordering sequences to transaction_versions and posting_versions,
-- and add the sequence-aware boundary column to reconciliation_checkpoints.
--
-- The append-only triggers on transaction_versions and posting_versions block all
-- UPDATEs unconditionally. The migration must drop them, add the columns with a
-- temporary DEFAULT so existing rows can be inserted, backfill deterministically,
-- remove the DEFAULT, then restore the triggers.
--
-- Backfill strategy: rank rows by their IDs within each scope (this preserves
-- the existing visible order which already used IDs as tie-breakers). We use
-- DENSE_RANK over the *logical* entity IDs (transaction_id, posting_line_id)
-- rather than version IDs so multiple historical versions of the same logical row
-- receive the same initial position — only current rows need uniqueness, and the
-- service enforces that at write time.

-- +goose StatementBegin
DROP TRIGGER IF EXISTS transaction_versions_no_update;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TRIGGER IF EXISTS posting_versions_no_update;
-- +goose StatementEnd

-- Add the column with a default so SQLite accepts the ALTER TABLE.
ALTER TABLE transaction_versions
  ADD COLUMN transaction_day_sequence INTEGER NOT NULL DEFAULT 0;

ALTER TABLE posting_versions
  ADD COLUMN account_day_sequence INTEGER NOT NULL DEFAULT 0;

-- Backfill transaction_day_sequence.
-- Rank transaction_id within (book_id, transaction_date), most-recent-first by
-- ID to match the existing sort direction. We assign sequence 1 to the row with
-- the smallest transaction_id (earliest inserted) so the visible order is
-- oldest-first within a date, matching the DESC ordering tie-breaker: after sort
-- the largest-ID row appears first (sequence 1).
--
-- Implementation: DENSE_RANK() is not available in a simple UPDATE...WHERE in
-- older SQLite; use a correlated subcount instead.
-- +goose StatementBegin
UPDATE transaction_versions
SET transaction_day_sequence = (
  SELECT COUNT(DISTINCT other.transaction_id)
  FROM transaction_versions other
  WHERE other.book_id = transaction_versions.book_id
    AND other.transaction_date = transaction_versions.transaction_date
    AND other.transaction_id <= transaction_versions.transaction_id
);
-- +goose StatementEnd

-- Backfill account_day_sequence.
-- Rank posting_line_id within (book_id, account_id, entry_date via journal_entry).
-- +goose StatementBegin
UPDATE posting_versions
SET account_day_sequence = (
  SELECT COUNT(DISTINCT other.posting_line_id)
  FROM posting_versions other
  JOIN journal_entries other_je ON other_je.id = other.journal_entry_id
  JOIN journal_entries self_je  ON self_je.id  = posting_versions.journal_entry_id
  WHERE other.book_id     = posting_versions.book_id
    AND other.account_id  = posting_versions.account_id
    AND other_je.entry_date = self_je.entry_date
    AND other.posting_line_id <= posting_versions.posting_line_id
);
-- +goose StatementEnd

-- Add the statement_account_sequence column to reconciliation_checkpoints.
-- 0 means "before every posting on statement_date".
ALTER TABLE reconciliation_checkpoints
  ADD COLUMN statement_account_sequence INTEGER NOT NULL DEFAULT 0;

-- Backfill: set to the greatest account_day_sequence among the postings that
-- were selected into each checkpoint whose entry_date equals statement_date,
-- or 0 if no such postings exist.
-- +goose StatementBegin
UPDATE reconciliation_checkpoints
SET statement_account_sequence = COALESCE((
  SELECT MAX(pv.account_day_sequence)
  FROM reconciliation_checkpoint_postings rcp
  JOIN posting_versions pv ON pv.id = rcp.posting_version_id
  WHERE rcp.checkpoint_id = reconciliation_checkpoints.id
    AND rcp.entry_date = reconciliation_checkpoints.statement_date
), 0);
-- +goose StatementEnd

-- Add a supporting index for the new cursor tuple on the list query.
CREATE INDEX IF NOT EXISTS transaction_versions_book_date_seq_idx
  ON transaction_versions (book_id, transaction_date DESC, transaction_day_sequence DESC, id DESC);

CREATE INDEX IF NOT EXISTS posting_versions_account_date_seq_idx
  ON posting_versions (book_id, account_id, account_day_sequence DESC, id DESC);

-- Restore the append-only UPDATE trigger for transaction_versions.
-- +goose StatementBegin
CREATE TRIGGER transaction_versions_no_update
BEFORE UPDATE ON transaction_versions
BEGIN
  SELECT RAISE(ABORT, 'transaction_versions rows are append-only');
END;
-- +goose StatementEnd

-- Restore the append-only UPDATE trigger for posting_versions.
-- +goose StatementBegin
CREATE TRIGGER posting_versions_no_update
BEFORE UPDATE ON posting_versions
BEGIN
  SELECT RAISE(ABORT, 'posting_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TRIGGER IF EXISTS transaction_versions_no_update;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TRIGGER IF EXISTS posting_versions_no_update;
-- +goose StatementEnd

DROP INDEX IF EXISTS posting_versions_account_date_seq_idx;
DROP INDEX IF EXISTS transaction_versions_book_date_seq_idx;

ALTER TABLE reconciliation_checkpoints DROP COLUMN statement_account_sequence;
ALTER TABLE posting_versions           DROP COLUMN account_day_sequence;
ALTER TABLE transaction_versions       DROP COLUMN transaction_day_sequence;

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS transaction_versions_no_update
BEFORE UPDATE ON transaction_versions
BEGIN
  SELECT RAISE(ABORT, 'transaction_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS posting_versions_no_update
BEFORE UPDATE ON posting_versions
BEGIN
  SELECT RAISE(ABORT, 'posting_versions rows are append-only');
END;
-- +goose StatementEnd
