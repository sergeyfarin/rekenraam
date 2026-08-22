-- +goose Up
-- Voiding a price observation is a user operation, so it needs its own
-- audit_events reference. created_audit_event_id records the insert; without
-- this column a void's who/when/why would be an orphaned audit row (T-37).
ALTER TABLE price_observations
  ADD COLUMN voided_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS price_observations_voided_idx
  ON price_observations (book_id, voided_at)
  WHERE voided_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS price_observations_voided_idx;
ALTER TABLE price_observations DROP COLUMN voided_audit_event_id;
