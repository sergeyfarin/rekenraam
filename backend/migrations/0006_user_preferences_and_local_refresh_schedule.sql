-- +goose Up
CREATE TABLE IF NOT EXISTS user_preferences (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE UNIQUE,
  time_zone TEXT NOT NULL DEFAULT 'UTC',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

INSERT INTO user_preferences (
  user_id, time_zone, created_at, created_by_user_id, updated_at, updated_by_user_id
)
SELECT id, 'UTC', strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), id, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), id
FROM users
WHERE is_owner = 1
  AND NOT EXISTS (
    SELECT 1
    FROM user_preferences
    WHERE user_preferences.user_id = users.id
  );

ALTER TABLE pricing_policies
  ADD COLUMN refresh_hour_local INTEGER NOT NULL DEFAULT 4 CHECK (refresh_hour_local BETWEEN 0 AND 23);

ALTER TABLE pricing_policies
  ADD COLUMN refresh_minute_local INTEGER NOT NULL DEFAULT 0 CHECK (refresh_minute_local BETWEEN 0 AND 59);

UPDATE pricing_policies
SET refresh_hour_local = refresh_hour_utc,
    refresh_minute_local = refresh_minute_utc;

-- +goose Down
DROP TABLE IF EXISTS user_preferences;
