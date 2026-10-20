-- +goose Up
-- #334: hash-only, bounded-scope owner API credentials and attributed events.
CREATE TABLE authentication_events_v2 (
  id INTEGER PRIMARY KEY,
  occurred_at TEXT NOT NULL,
  event_type TEXT NOT NULL CHECK (
    event_type IN (
      'login_succeeded', 'login_failed', 'login_blocked', 'logout',
      'mfa_enrollment_started', 'mfa_activated', 'mfa_disabled',
      'mfa_recovery_codes_regenerated', 'trusted_device_revoked',
      'owner_access_recovered', 'token_created', 'token_revoked',
      'token_authenticated', 'token_denied'
    )
  ),
  outcome TEXT NOT NULL CHECK (outcome IN ('success', 'failure')),
  username TEXT NOT NULL DEFAULT '',
  user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
  auth_session_id INTEGER,
  client_ip TEXT NOT NULL DEFAULT '',
  failure_reason TEXT NOT NULL DEFAULT '',
  request_id TEXT NOT NULL DEFAULT '',
  origin TEXT NOT NULL DEFAULT 'browser_api' CHECK (origin IN ('browser_api', 'cli_recovery', 'personal_access_token')),
  second_factor TEXT NOT NULL DEFAULT '' CHECK (second_factor IN ('', 'totp', 'recovery_code')),
  metadata_json TEXT NOT NULL DEFAULT '{}' CHECK (
    json_valid(metadata_json) AND json_type(metadata_json) = 'object'
  ),
  CHECK (second_factor = '' OR event_type = 'login_succeeded'),
  CHECK ((origin = 'cli_recovery') = (event_type = 'owner_access_recovered'))
);

INSERT INTO authentication_events_v2 (
  id, occurred_at, event_type, outcome, username, user_id, auth_session_id,
  client_ip, failure_reason, request_id, origin, second_factor, metadata_json
)
SELECT id, occurred_at, event_type, outcome, username, user_id, auth_session_id,
  client_ip, failure_reason, request_id, origin, second_factor, metadata_json
FROM authentication_events;

DROP INDEX IF EXISTS authentication_events_client_ip_idx;
DROP INDEX IF EXISTS authentication_events_occurred_idx;
DROP TABLE authentication_events;
ALTER TABLE authentication_events_v2 RENAME TO authentication_events;

CREATE INDEX authentication_events_occurred_idx
  ON authentication_events (occurred_at DESC, id DESC);

CREATE INDEX authentication_events_client_ip_idx
  ON authentication_events (client_ip, occurred_at DESC)
  WHERE client_ip <> '';


CREATE TABLE personal_access_tokens (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  name TEXT NOT NULL CHECK (length(CAST(name AS BLOB)) BETWEEN 1 AND 80),
  token_hash TEXT NOT NULL UNIQUE CHECK (length(token_hash) = 64),
  scopes_json TEXT NOT NULL CHECK (json_valid(scopes_json) AND json_type(scopes_json) = 'array' AND json_array_length(scopes_json) BETWEEN 1 AND 2),
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL CHECK (expires_at > created_at),
  revoked_at TEXT,
  last_used_at TEXT
);
CREATE INDEX personal_access_tokens_owner_idx ON personal_access_tokens(user_id, id DESC);

-- +goose Down
-- Refuse loss of credentials or token security history. Use verified backup rollback.
CREATE TEMP TABLE token_downgrade_guard (empty INTEGER CHECK (empty = 1));
INSERT INTO token_downgrade_guard SELECT NOT EXISTS (SELECT 1 FROM personal_access_tokens)
  AND NOT EXISTS (SELECT 1 FROM authentication_events WHERE event_type LIKE 'token_%' OR origin = 'personal_access_token');
DROP TABLE token_downgrade_guard;
DROP TABLE personal_access_tokens;

CREATE TABLE authentication_events_v2 (
  id INTEGER PRIMARY KEY,
  occurred_at TEXT NOT NULL,
  event_type TEXT NOT NULL CHECK (
    event_type IN (
      'login_succeeded', 'login_failed', 'login_blocked', 'logout',
      'mfa_enrollment_started', 'mfa_activated', 'mfa_disabled',
      'mfa_recovery_codes_regenerated', 'trusted_device_revoked',
      'owner_access_recovered'
    )
  ),
  outcome TEXT NOT NULL CHECK (outcome IN ('success', 'failure')),
  username TEXT NOT NULL DEFAULT '',
  user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
  auth_session_id INTEGER,
  client_ip TEXT NOT NULL DEFAULT '',
  failure_reason TEXT NOT NULL DEFAULT '',
  request_id TEXT NOT NULL DEFAULT '',
  origin TEXT NOT NULL DEFAULT 'browser_api' CHECK (origin IN ('browser_api', 'cli_recovery')),
  second_factor TEXT NOT NULL DEFAULT '' CHECK (second_factor IN ('', 'totp', 'recovery_code')),
  metadata_json TEXT NOT NULL DEFAULT '{}' CHECK (
    json_valid(metadata_json) AND json_type(metadata_json) = 'object'
  ),
  CHECK (second_factor = '' OR event_type = 'login_succeeded'),
  CHECK ((origin = 'cli_recovery') = (event_type = 'owner_access_recovered'))
);

INSERT INTO authentication_events_v2 (
  id, occurred_at, event_type, outcome, username, user_id, auth_session_id,
  client_ip, failure_reason, request_id, origin, second_factor, metadata_json
)
SELECT id, occurred_at, event_type, outcome, username, user_id, auth_session_id,
  client_ip, failure_reason, request_id, origin, second_factor, metadata_json
FROM authentication_events;

DROP INDEX IF EXISTS authentication_events_client_ip_idx;
DROP INDEX IF EXISTS authentication_events_occurred_idx;
DROP TABLE authentication_events;
ALTER TABLE authentication_events_v2 RENAME TO authentication_events;

CREATE INDEX authentication_events_occurred_idx
  ON authentication_events (occurred_at DESC, id DESC);

CREATE INDEX authentication_events_client_ip_idx
  ON authentication_events (client_ip, occurred_at DESC)
  WHERE client_ip <> '';
