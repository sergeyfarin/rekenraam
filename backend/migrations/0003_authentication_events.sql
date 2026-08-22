-- +goose Up
-- Durable record of successful and failed authentication attempts so an
-- operator can spot a brute-force run and reconstruct an incident (S-07).
--
-- Privacy posture, deliberate:
--   * No password material, no session token, not even the token hash — the
--     session id is enough to correlate with auth_sessions.
--   * The attempted username is stored because "which account is being
--     guessed" is the whole point; it has already passed username-shape
--     validation before an event is written.
--   * client_ip is the proxy-aware client address resolved by the API layer,
--     not the raw peer address, so events behind a reverse proxy name the
--     real client.
--   * Rows are pruned by the existing session-cleanup loop; this is an
--     operational log with a retention window, not a permanent archive.
CREATE TABLE IF NOT EXISTS authentication_events (
  id INTEGER PRIMARY KEY,
  occurred_at TEXT NOT NULL,
  event_type TEXT NOT NULL CHECK (
    event_type IN ('login_succeeded', 'login_failed', 'login_blocked', 'logout')
  ),
  outcome TEXT NOT NULL CHECK (outcome IN ('success', 'failure')),
  username TEXT NOT NULL DEFAULT '',
  user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
  auth_session_id INTEGER REFERENCES auth_sessions(id) ON DELETE SET NULL,
  client_ip TEXT NOT NULL DEFAULT '',
  failure_reason TEXT NOT NULL DEFAULT '',
  request_id TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS authentication_events_occurred_idx
  ON authentication_events (occurred_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS authentication_events_client_ip_idx
  ON authentication_events (client_ip, occurred_at DESC)
  WHERE client_ip <> '';

-- +goose Down
DROP INDEX IF EXISTS authentication_events_client_ip_idx;
DROP INDEX IF EXISTS authentication_events_occurred_idx;
DROP TABLE IF EXISTS authentication_events;
