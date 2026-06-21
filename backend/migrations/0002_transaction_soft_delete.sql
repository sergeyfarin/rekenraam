-- +goose Up
ALTER TABLE transactions ADD COLUMN deleted_at TEXT;
ALTER TABLE transactions ADD COLUMN deleted_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT;
ALTER TABLE transactions ADD COLUMN deleted_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT;
ALTER TABLE transactions ADD COLUMN delete_reason TEXT;

CREATE INDEX transactions_deleted_idx
  ON transactions (book_id, deleted_at, id)
  WHERE deleted_at IS NOT NULL;

CREATE TABLE transaction_deletion_events (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE RESTRICT,
  action TEXT NOT NULL CHECK (action IN ('soft_delete', 'restore')),
  occurred_at TEXT NOT NULL,
  actor_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  audit_event_id INTEGER NOT NULL REFERENCES audit_events(id) ON DELETE RESTRICT,
  reason TEXT NOT NULL
);

CREATE INDEX transaction_deletion_events_transaction_idx
  ON transaction_deletion_events (transaction_id, id);

-- +goose StatementBegin
CREATE TRIGGER transaction_deletion_events_no_update
BEFORE UPDATE ON transaction_deletion_events
BEGIN
  SELECT RAISE(ABORT, 'transaction_deletion_events rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER transaction_deletion_events_no_delete
BEFORE DELETE ON transaction_deletion_events
BEGIN
  SELECT RAISE(ABORT, 'transaction_deletion_events rows are append-only');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS transaction_deletion_events_no_delete;
DROP TRIGGER IF EXISTS transaction_deletion_events_no_update;
DROP INDEX IF EXISTS transaction_deletion_events_transaction_idx;
DROP TABLE IF EXISTS transaction_deletion_events;
DROP INDEX IF EXISTS transactions_deleted_idx;
ALTER TABLE transactions DROP COLUMN delete_reason;
ALTER TABLE transactions DROP COLUMN deleted_audit_event_id;
ALTER TABLE transactions DROP COLUMN deleted_by_user_id;
ALTER TABLE transactions DROP COLUMN deleted_at;
