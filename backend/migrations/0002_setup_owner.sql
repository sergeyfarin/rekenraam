-- +goose Up
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  is_owner INTEGER NOT NULL CHECK (is_owner IN (0, 1)),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS users_one_owner_idx
  ON users (is_owner)
  WHERE is_owner = 1;

CREATE TABLE IF NOT EXISTS auth_sessions (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS setup_steps (
  step_key TEXT PRIMARY KEY,
  completed_at TEXT
);

INSERT OR IGNORE INTO setup_steps (step_key, completed_at)
VALUES
  ('owner', NULL),
  ('book', NULL),
  ('currencies', NULL),
  ('system_accounts', NULL),
  ('categories', NULL);

-- +goose Down
DROP TABLE IF EXISTS auth_sessions;
DROP INDEX IF EXISTS users_one_owner_idx;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS setup_steps;