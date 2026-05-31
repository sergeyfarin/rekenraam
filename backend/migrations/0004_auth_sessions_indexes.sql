-- +goose Up
-- Index to support efficient lookup of all sessions for a given user.
-- Required when revoking all sessions on password change or account recovery.
CREATE INDEX IF NOT EXISTS auth_sessions_user_id_idx
  ON auth_sessions (user_id);

-- Index to support efficient cleanup queries for expired and revoked sessions.
-- Expired session rows accumulate over time and should be pruned periodically.
-- A periodic cleanup query is: DELETE FROM auth_sessions WHERE revoked_at IS NOT NULL OR expires_at <= ?
CREATE INDEX IF NOT EXISTS auth_sessions_expires_revoked_idx
  ON auth_sessions (expires_at, revoked_at);

-- +goose Down
DROP INDEX IF EXISTS auth_sessions_expires_revoked_idx;
DROP INDEX IF EXISTS auth_sessions_user_id_idx;
