-- +goose Up
-- A discarded generated draft becomes a skipped tombstone. Keep an explicit
-- audit link after clearing its live transaction FK, so discard remains
-- attributable even though the never-posted draft itself is removed.
ALTER TABLE recurring_occurrences ADD COLUMN last_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT;

-- +goose Down
ALTER TABLE recurring_occurrences DROP COLUMN last_audit_event_id;
