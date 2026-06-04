-- +goose Up
CREATE TABLE IF NOT EXISTS audit_events (
  id INTEGER PRIMARY KEY,
  book_id INTEGER REFERENCES books(id) ON DELETE RESTRICT,
  actor_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  auth_session_id INTEGER REFERENCES auth_sessions(id) ON DELETE SET NULL,
  occurred_at TEXT NOT NULL,
  request_id TEXT,
  origin_type TEXT NOT NULL CHECK (origin_type IN ('browser_api', 'setup', 'cli_recovery', 'import', 'system_seed', 'scheduled', 'internal')),
  operation TEXT NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS audit_events_book_occurred_idx
  ON audit_events (book_id, occurred_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS audit_events_actor_occurred_idx
  ON audit_events (actor_user_id, occurred_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS audit_events_request_idx
  ON audit_events (request_id)
  WHERE request_id IS NOT NULL;

ALTER TABLE books ADD COLUMN created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT;
ALTER TABLE books ADD COLUMN updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT;

ALTER TABLE commodities ADD COLUMN created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT;
ALTER TABLE commodity_versions ADD COLUMN change_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT;

ALTER TABLE institutions ADD COLUMN created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT;
ALTER TABLE institution_versions ADD COLUMN change_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT;

ALTER TABLE accounts ADD COLUMN created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT;
ALTER TABLE account_versions ADD COLUMN change_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT;

-- +goose Down
ALTER TABLE account_versions DROP COLUMN change_audit_event_id;
ALTER TABLE accounts DROP COLUMN created_audit_event_id;

ALTER TABLE institution_versions DROP COLUMN change_audit_event_id;
ALTER TABLE institutions DROP COLUMN created_audit_event_id;

ALTER TABLE commodity_versions DROP COLUMN change_audit_event_id;
ALTER TABLE commodities DROP COLUMN created_audit_event_id;

ALTER TABLE books DROP COLUMN updated_audit_event_id;
ALTER TABLE books DROP COLUMN created_audit_event_id;

DROP INDEX IF EXISTS audit_events_request_idx;
DROP INDEX IF EXISTS audit_events_actor_occurred_idx;
DROP INDEX IF EXISTS audit_events_book_occurred_idx;
DROP TABLE IF EXISTS audit_events;
