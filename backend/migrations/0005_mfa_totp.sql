-- +goose Up
-- Multi-factor authentication (S-06), the last gate on deploying real
-- financial data to the public internet. The second factor is TOTP
-- (RFC 6238) plus single-use recovery codes.
--
-- Why TOTP and not WebAuthn: this is a single-owner, self-hosted app that is
-- routinely reached over a LAN address or a private hostname, where WebAuthn's
-- origin binding is awkward and its recovery story for an owner who loses
-- their only authenticator is worse. An authenticator app works everywhere the
-- app does.

-- One enrollment per user. 'pending' means the secret has been issued but no
-- code has proved the user actually stored it; only 'active' gates a login, so
-- an abandoned enrollment can never lock anyone out.
CREATE TABLE IF NOT EXISTS user_mfa_totp (
  user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  -- secretbox (AES-256-GCM) ciphertext of the base32 shared secret. The
  -- secret is a credential-equivalent: anyone holding it can mint valid
  -- codes forever, so it is never stored in the clear. Enrollment therefore
  -- requires REKENRAAM_SECRET_KEY, exactly like online import connections.
  secret_ciphertext TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending', 'active')),
  created_at TEXT NOT NULL,
  activated_at TEXT,
  -- The RFC 6238 counter of the last accepted code. A code stays valid for
  -- its whole 30-second step, so without refusing steps at or below this one
  -- a shoulder-surfed or intercepted code could be replayed inside the
  -- window.
  last_used_step INTEGER
);

-- Recovery codes are the answer to "my phone is gone". Single use, hashed at
-- rest, and regenerated as a set: showing a used code again would be a lie
-- about what still works.
--
-- SHA-256 without a slow KDF is deliberate and matches session tokens: these
-- are 100+ bits of server-generated randomness, not user-chosen passwords, so
-- there is nothing for a slow hash to protect against.
CREATE TABLE IF NOT EXISTS user_mfa_recovery_codes (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  code_hash TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL,
  used_at TEXT,
  -- The client IP that spent the code, so a recovery-code login is
  -- recognisable when reviewing the authentication log.
  used_client_ip TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS user_mfa_recovery_codes_user_idx
  ON user_mfa_recovery_codes (user_id, used_at);

-- A password that verified but has not yet met the second factor. This is the
-- half-authenticated state, and it is deliberately a durable row rather than
-- anything the browser could forge: it carries no session, grants nothing, and
-- expires in minutes.
CREATE TABLE IF NOT EXISTS login_mfa_challenges (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  consumed_at TEXT,
  client_ip TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS login_mfa_challenges_expiry_idx
  ON login_mfa_challenges (expires_at);

-- +goose Down
DROP INDEX IF EXISTS login_mfa_challenges_expiry_idx;
DROP TABLE IF EXISTS login_mfa_challenges;
DROP INDEX IF EXISTS user_mfa_recovery_codes_user_idx;
DROP TABLE IF EXISTS user_mfa_recovery_codes;
DROP TABLE IF EXISTS user_mfa_totp;
