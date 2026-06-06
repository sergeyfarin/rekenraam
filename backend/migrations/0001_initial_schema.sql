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
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  revoked_at TEXT
);

CREATE INDEX IF NOT EXISTS auth_sessions_user_id_idx
  ON auth_sessions (user_id);

CREATE INDEX IF NOT EXISTS auth_sessions_expires_revoked_idx
  ON auth_sessions (expires_at, revoked_at);

CREATE TABLE IF NOT EXISTS audit_events (
  id INTEGER PRIMARY KEY,
  book_id INTEGER REFERENCES books(id) ON DELETE RESTRICT,
  actor_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  auth_session_id INTEGER REFERENCES auth_sessions(id) ON DELETE SET NULL,
  occurred_at TEXT NOT NULL CHECK (
    length(occurred_at) = 20
    AND occurred_at GLOB '????-??-??T??:??:??Z'
  ),
  request_id TEXT,
  origin_type TEXT NOT NULL CHECK (origin_type IN ('browser_api', 'setup', 'cli_recovery', 'import', 'system_seed', 'scheduled', 'internal')),
  operation TEXT NOT NULL CHECK (length(trim(operation)) > 0 AND operation = trim(operation)),
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

CREATE TABLE IF NOT EXISTS setup_steps (
  step_key TEXT PRIMARY KEY,
  completed_at TEXT,
  completed_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

INSERT OR IGNORE INTO setup_steps (step_key, completed_at)
VALUES
  ('owner', NULL),
  ('book', NULL),
  ('currencies', NULL),
  ('system_accounts', NULL),
  ('categories', NULL);

CREATE TABLE IF NOT EXISTS login_throttles (
  scope_type TEXT NOT NULL,
  scope_key TEXT NOT NULL,
  failed_attempts INTEGER NOT NULL DEFAULT 0,
  blocked_until TEXT,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (scope_type, scope_key)
);

CREATE TABLE IF NOT EXISTS books (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  owner_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  code TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  default_currency_commodity_id INTEGER,
  updated_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS tags (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  name TEXT NOT NULL CHECK (length(trim(name)) > 0 AND name = trim(name)),
  kind TEXT NOT NULL CHECK (kind IN ('project', 'person', 'flag', 'place', 'custom')),
  color TEXT CHECK (color IS NULL OR color GLOB '#[0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f]'),
  icon TEXT CHECK (icon IS NULL OR (length(trim(icon)) > 0 AND icon = trim(icon))),
  status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_request_id TEXT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS tags_book_status_kind_idx
  ON tags (book_id, status, kind, name COLLATE NOCASE);

CREATE UNIQUE INDEX IF NOT EXISTS tags_active_name_kind_idx
  ON tags (book_id, kind, name COLLATE NOCASE)
  WHERE status = 'active';

CREATE TABLE IF NOT EXISTS commodities (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  code TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('currency', 'security', 'crypto', 'reward', 'commodity')),
  is_builtin INTEGER NOT NULL CHECK (is_builtin IN (0, 1)),
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_request_id TEXT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  UNIQUE (book_id, code)
);

CREATE INDEX IF NOT EXISTS commodities_book_kind_idx
  ON commodities (book_id, kind, code);

CREATE TABLE IF NOT EXISTS commodity_versions (
  id INTEGER PRIMARY KEY,
  commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  version_seq INTEGER NOT NULL CHECK (version_seq > 0),
  effective_from TEXT NOT NULL CHECK (effective_from GLOB '????-??-??'),
  recorded_at TEXT NOT NULL,
  changed_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  change_reason TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
  symbol TEXT NOT NULL,
  display_symbol TEXT NOT NULL,
  name TEXT NOT NULL,
  standard_scale INTEGER NOT NULL CHECK (standard_scale BETWEEN 0 AND 12),
  max_quantity_scale INTEGER NOT NULL CHECK (max_quantity_scale BETWEEN 0 AND 12 AND max_quantity_scale >= standard_scale),
  metadata_json TEXT NOT NULL DEFAULT '{}',
  change_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  UNIQUE (commodity_id, version_seq)
);

CREATE INDEX IF NOT EXISTS commodity_versions_commodity_seq_idx
  ON commodity_versions (commodity_id, version_seq DESC);

CREATE VIEW IF NOT EXISTS current_commodity_versions AS
SELECT cv.*
FROM commodity_versions cv
WHERE cv.id = (
  SELECT current_cv.id
  FROM commodity_versions current_cv
  WHERE current_cv.commodity_id = cv.commodity_id
    AND current_cv.effective_from <= date('now')
  ORDER BY current_cv.effective_from DESC, current_cv.version_seq DESC
  LIMIT 1
);

CREATE TABLE IF NOT EXISTS institutions (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_request_id TEXT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS institutions_book_idx
  ON institutions (book_id);

CREATE TABLE IF NOT EXISTS institution_versions (
  id INTEGER PRIMARY KEY,
  institution_id INTEGER NOT NULL REFERENCES institutions(id) ON DELETE RESTRICT,
  version_seq INTEGER NOT NULL CHECK (version_seq > 0),
  effective_from TEXT NOT NULL CHECK (effective_from GLOB '????-??-??'),
  recorded_at TEXT NOT NULL,
  changed_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  change_reason TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
  name TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('bank', 'credit_union', 'brokerage', 'card_issuer', 'lender', 'insurance', 'employer', 'rewards_program', 'government', 'other')),
  country_code TEXT,
  website TEXT,
  address_json TEXT NOT NULL DEFAULT '{}',
  comment_markdown TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  change_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  UNIQUE (institution_id, version_seq)
);

CREATE INDEX IF NOT EXISTS institution_versions_institution_seq_idx
  ON institution_versions (institution_id, version_seq DESC);

CREATE VIEW IF NOT EXISTS current_institution_versions AS
SELECT iv.*
FROM institution_versions iv
WHERE iv.id = (
  SELECT current_iv.id
  FROM institution_versions current_iv
  WHERE current_iv.institution_id = iv.institution_id
    AND current_iv.effective_from <= date('now')
  ORDER BY current_iv.effective_from DESC, current_iv.version_seq DESC
  LIMIT 1
);

CREATE TABLE IF NOT EXISTS account_kinds (
  code TEXT PRIMARY KEY,
  account_class TEXT NOT NULL CHECK (account_class IN ('asset', 'liability', 'equity', 'income', 'expense')),
  base_kind TEXT NOT NULL,
  is_builtin INTEGER NOT NULL CHECK (is_builtin IN (0, 1)),
  is_user_assignable INTEGER NOT NULL CHECK (is_user_assignable IN (0, 1)),
  display_key TEXT NOT NULL,
  sort_order INTEGER NOT NULL,
  UNIQUE (code, account_class)
);

INSERT OR IGNORE INTO account_kinds (
  code,
  account_class,
  base_kind,
  is_builtin,
  is_user_assignable,
  display_key,
  sort_order
) VALUES
  ('cash', 'asset', 'cash', 1, 1, 'account_kind_cash', 100),
  ('checking', 'asset', 'bank_account', 1, 1, 'account_kind_checking', 120),
  ('savings', 'asset', 'bank_account', 1, 1, 'account_kind_savings', 130),
  ('term_deposit', 'asset', 'bank_account', 1, 1, 'account_kind_term_deposit', 140),
  ('brokerage', 'asset', 'investment_container', 1, 1, 'account_kind_brokerage', 200),
  ('brokerage_cash', 'asset', 'investment_cash', 1, 1, 'account_kind_brokerage_cash', 210),
  ('security_holding', 'asset', 'security_holding', 1, 1, 'account_kind_security_holding', 220),
  ('crypto_wallet', 'asset', 'digital_asset', 1, 1, 'account_kind_crypto_wallet', 230),
  ('property', 'asset', 'tangible_asset', 1, 1, 'account_kind_property', 300),
  ('vehicle', 'asset', 'tangible_asset', 1, 1, 'account_kind_vehicle', 310),
  ('rewards_balance', 'asset', 'non_cash_balance', 1, 1, 'account_kind_rewards_balance', 400),
  ('receivable', 'asset', 'receivable', 1, 1, 'account_kind_receivable', 500),
  ('other_asset', 'asset', 'other', 1, 1, 'account_kind_other_asset', 900),
  ('credit_card', 'liability', 'revolving_credit', 1, 1, 'account_kind_credit_card', 100),
  ('line_of_credit', 'liability', 'revolving_credit', 1, 1, 'account_kind_line_of_credit', 110),
  ('loan', 'liability', 'loan', 1, 1, 'account_kind_loan', 200),
  ('mortgage', 'liability', 'loan', 1, 1, 'account_kind_mortgage', 210),
  ('tax_liability', 'liability', 'payable', 1, 1, 'account_kind_tax_liability', 300),
  ('payable', 'liability', 'payable', 1, 1, 'account_kind_payable', 310),
  ('other_liability', 'liability', 'other', 1, 1, 'account_kind_other_liability', 900),
  ('equity', 'equity', 'equity', 1, 1, 'account_kind_equity', 100),
  ('income', 'income', 'income', 1, 1, 'account_kind_income', 100),
  ('expense', 'expense', 'expense', 1, 1, 'account_kind_expense', 100);

CREATE TABLE IF NOT EXISTS accounts (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  system_role TEXT CHECK (
    system_role IS NULL OR system_role IN (
      'opening_balance',
      'import_imbalance',
      'retained_earnings',
      'unassigned_income',
      'unassigned_expense'
    )
  ),
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_request_id TEXT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS accounts_system_role_idx
  ON accounts (book_id, system_role)
  WHERE system_role IS NOT NULL;

CREATE INDEX IF NOT EXISTS accounts_book_idx
  ON accounts (book_id);

CREATE TABLE IF NOT EXISTS account_versions (
  id INTEGER PRIMARY KEY,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  version_seq INTEGER NOT NULL CHECK (version_seq > 0),
  effective_from TEXT NOT NULL CHECK (effective_from GLOB '????-??-??'),
  recorded_at TEXT NOT NULL,
  changed_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  change_reason TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('active', 'closed', 'archived')),
  opened_on TEXT NOT NULL CHECK (opened_on GLOB '????-??-??'),
  closed_on TEXT CHECK (closed_on IS NULL OR closed_on GLOB '????-??-??'),
  code TEXT,
  name TEXT,
  account_class TEXT NOT NULL CHECK (account_class IN ('asset', 'liability', 'equity', 'income', 'expense')),
  account_kind TEXT NOT NULL,
  parent_account_id INTEGER REFERENCES accounts(id) ON DELETE RESTRICT,
  institution_id INTEGER REFERENCES institutions(id) ON DELETE RESTRICT,
  country_code TEXT,
  default_commodity_id INTEGER REFERENCES commodities(id) ON DELETE RESTRICT,
  quantity_scale_override INTEGER CHECK (quantity_scale_override IS NULL OR quantity_scale_override BETWEEN 0 AND 12),
  allows_postings INTEGER NOT NULL CHECK (allows_postings IN (0, 1)),
  number_last4 TEXT,
  external_ref_hint TEXT,
  comment_markdown TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  change_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  UNIQUE (account_id, version_seq),
  CHECK (closed_on IS NULL OR closed_on >= opened_on),
  CHECK (
    (status = 'active' AND closed_on IS NULL)
    OR (status IN ('closed', 'archived') AND closed_on IS NOT NULL)
  ),
  FOREIGN KEY (account_kind, account_class) REFERENCES account_kinds(code, account_class) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS account_versions_account_seq_idx
  ON account_versions (account_id, version_seq DESC);

CREATE INDEX IF NOT EXISTS account_versions_parent_idx
  ON account_versions (parent_account_id);

CREATE INDEX IF NOT EXISTS account_versions_institution_idx
  ON account_versions (institution_id);

CREATE INDEX IF NOT EXISTS account_versions_default_commodity_idx
  ON account_versions (default_commodity_id);

CREATE VIEW IF NOT EXISTS current_account_versions AS
SELECT av.*
FROM account_versions av
WHERE av.id = (
  SELECT current_av.id
  FROM account_versions current_av
  WHERE current_av.account_id = av.account_id
    AND current_av.effective_from <= date('now')
  ORDER BY current_av.effective_from DESC, current_av.version_seq DESC
  LIMIT 1
);

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS books_default_currency_must_exist_on_insert
BEFORE INSERT ON books
WHEN NEW.default_currency_commodity_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM commodities
    WHERE id = NEW.default_currency_commodity_id
      AND book_id = NEW.id
      AND kind = 'currency'
  )
BEGIN
  SELECT RAISE(ABORT, 'book default currency must reference a currency in the same book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS books_default_currency_must_exist
BEFORE UPDATE OF default_currency_commodity_id ON books
WHEN NEW.default_currency_commodity_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM commodities
    WHERE id = NEW.default_currency_commodity_id
      AND book_id = NEW.id
      AND kind = 'currency'
  )
BEGIN
  SELECT RAISE(ABORT, 'book default currency must reference a currency in the same book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS commodity_versions_no_update
BEFORE UPDATE ON commodity_versions
BEGIN
  SELECT RAISE(ABORT, 'commodity_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS commodity_versions_no_delete
BEFORE DELETE ON commodity_versions
BEGIN
  SELECT RAISE(ABORT, 'commodity_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS institution_versions_no_update
BEFORE UPDATE ON institution_versions
BEGIN
  SELECT RAISE(ABORT, 'institution_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS institution_versions_no_delete
BEFORE DELETE ON institution_versions
BEGIN
  SELECT RAISE(ABORT, 'institution_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS accounts_system_fields_no_update
BEFORE UPDATE OF system_role ON accounts
BEGIN
  SELECT RAISE(ABORT, 'account system identity fields are immutable');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS account_versions_no_update
BEFORE UPDATE ON account_versions
BEGIN
  SELECT RAISE(ABORT, 'account_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS account_versions_no_delete
BEFORE DELETE ON account_versions
BEGIN
  SELECT RAISE(ABORT, 'account_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS account_versions_no_delete;
DROP TRIGGER IF EXISTS account_versions_no_update;
DROP TRIGGER IF EXISTS accounts_system_fields_no_update;
DROP TRIGGER IF EXISTS institution_versions_no_delete;
DROP TRIGGER IF EXISTS institution_versions_no_update;
DROP TRIGGER IF EXISTS commodity_versions_no_delete;
DROP TRIGGER IF EXISTS commodity_versions_no_update;
DROP TRIGGER IF EXISTS books_default_currency_must_exist;
DROP TRIGGER IF EXISTS books_default_currency_must_exist_on_insert;

DROP INDEX IF EXISTS account_versions_default_commodity_idx;
DROP INDEX IF EXISTS account_versions_institution_idx;
DROP INDEX IF EXISTS account_versions_parent_idx;
DROP INDEX IF EXISTS account_versions_account_seq_idx;
DROP VIEW IF EXISTS current_account_versions;
DROP TABLE IF EXISTS account_versions;

DROP INDEX IF EXISTS accounts_book_idx;
DROP INDEX IF EXISTS accounts_system_role_idx;
DROP TABLE IF EXISTS accounts;

DROP TABLE IF EXISTS account_kinds;

DROP INDEX IF EXISTS institution_versions_institution_seq_idx;
DROP VIEW IF EXISTS current_institution_versions;
DROP TABLE IF EXISTS institution_versions;

DROP INDEX IF EXISTS institutions_book_idx;
DROP TABLE IF EXISTS institutions;

DROP INDEX IF EXISTS commodity_versions_commodity_seq_idx;
DROP VIEW IF EXISTS current_commodity_versions;
DROP TABLE IF EXISTS commodity_versions;

DROP INDEX IF EXISTS commodities_book_kind_idx;
DROP TABLE IF EXISTS commodities;

DROP INDEX IF EXISTS tags_active_name_kind_idx;
DROP INDEX IF EXISTS tags_book_status_kind_idx;
DROP TABLE IF EXISTS tags;

DROP TABLE IF EXISTS books;
DROP TABLE IF EXISTS login_throttles;
DROP TABLE IF EXISTS setup_steps;

DROP INDEX IF EXISTS audit_events_request_idx;
DROP INDEX IF EXISTS audit_events_actor_occurred_idx;
DROP INDEX IF EXISTS audit_events_book_occurred_idx;
DROP TABLE IF EXISTS audit_events;

DROP INDEX IF EXISTS auth_sessions_expires_revoked_idx;
DROP INDEX IF EXISTS auth_sessions_user_id_idx;
DROP TABLE IF EXISTS auth_sessions;

DROP INDEX IF EXISTS users_one_owner_idx;
DROP TABLE IF EXISTS users;
