-- +goose Up
CREATE TABLE auth_sessions_v3 (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  revoked_at TEXT
);

INSERT INTO auth_sessions_v3 (id, user_id, token_hash, created_at, expires_at, revoked_at)
SELECT
  id,
  user_id,
  token_hash,
  created_at,
  strftime('%Y-%m-%dT%H:%M:%SZ', datetime(created_at, '+30 days')),
  NULL
FROM auth_sessions;

DROP TABLE auth_sessions;
ALTER TABLE auth_sessions_v3 RENAME TO auth_sessions;

CREATE TABLE IF NOT EXISTS login_throttles (
  scope_type TEXT NOT NULL,
  scope_key TEXT NOT NULL,
  failed_attempts INTEGER NOT NULL DEFAULT 0,
  blocked_until TEXT,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (scope_type, scope_key)
);

-- +goose Down
DROP TABLE IF EXISTS login_throttles;

CREATE TABLE auth_sessions_v2 (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL
);

INSERT INTO auth_sessions_v2 (id, user_id, token_hash, created_at)
SELECT id, user_id, token_hash, created_at
FROM auth_sessions;

DROP TABLE auth_sessions;
ALTER TABLE auth_sessions_v2 RENAME TO auth_sessions;