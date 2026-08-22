-- +goose Up
-- Approved-device records that make the login throttle lockout-safe (S-04).
--
-- The problem: this app has exactly one owner, so its username is effectively
-- public. Any internet attacker could fail five logins and keep the
-- username-scoped throttle permanently engaged, locking the real owner out of
-- their own finances — the throttle became the denial of service.
--
-- A device that has previously completed a successful login carries a random
-- token; presenting it moves the attempt onto a throttle scope only that
-- device can fill. The token is a throttle-scope selector, NOT a credential:
-- it grants no access whatsoever, and a login still needs the password.
-- Only the hash is stored, exactly like session tokens.
CREATE TABLE IF NOT EXISTS login_trusted_devices (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL,
  last_used_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  revoked_at TEXT,
  -- The client IP the device was approved from, so the owner can recognise
  -- an entry when reviewing the list. Proxy-aware, same resolution as
  -- authentication_events.
  created_client_ip TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS login_trusted_devices_user_idx
  ON login_trusted_devices (user_id, revoked_at, expires_at);

-- +goose Down
DROP INDEX IF EXISTS login_trusted_devices_user_idx;
DROP TABLE IF EXISTS login_trusted_devices;
