-- +goose Up
-- Voiding a price observation is a user operation, and the audit model requires
-- every row an operation touches to reference the audit_events row that
-- explains who did it and why. price_observations carried created_audit_event_id
-- but had no equivalent for the void, so the link was unrecoverable from the row.
ALTER TABLE price_observations
  ADD COLUMN voided_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT;

-- +goose Down
-- SQLite cannot drop a column referenced by a foreign key without a table
-- rebuild; the column is nullable and unused when rolled back.
