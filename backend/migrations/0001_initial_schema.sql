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
  icon TEXT CHECK (
    icon IS NULL
    OR (
      length(icon) BETWEEN 1 AND 64
      AND icon = trim(icon)
      AND icon GLOB '[a-z]*'
      AND icon NOT GLOB '*[^a-z0-9_-]*'
    )
  ),
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
      'unassigned_expense',
      'transfer_clearing',
      'commodity_trading'
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

CREATE TABLE IF NOT EXISTS payees (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_request_id TEXT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS payees_book_idx
  ON payees (book_id);

CREATE TABLE IF NOT EXISTS payee_versions (
  id INTEGER PRIMARY KEY,
  payee_id INTEGER NOT NULL REFERENCES payees(id) ON DELETE RESTRICT,
  version_seq INTEGER NOT NULL CHECK (version_seq > 0),
  effective_from TEXT NOT NULL CHECK (effective_from GLOB '????-??-??'),
  recorded_at TEXT NOT NULL,
  changed_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  change_reason TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
  name TEXT NOT NULL CHECK (length(trim(name)) > 0 AND name = trim(name)),
  normalized_name TEXT NOT NULL CHECK (length(trim(normalized_name)) > 0 AND normalized_name = trim(normalized_name)),
  default_account_id INTEGER REFERENCES accounts(id) ON DELETE RESTRICT,
  default_category_account_id INTEGER REFERENCES accounts(id) ON DELETE RESTRICT,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  change_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  UNIQUE (payee_id, version_seq)
);

CREATE INDEX IF NOT EXISTS payee_versions_payee_seq_idx
  ON payee_versions (payee_id, version_seq DESC);

CREATE INDEX IF NOT EXISTS payee_versions_name_idx
  ON payee_versions (normalized_name, status);

CREATE VIEW IF NOT EXISTS current_payee_versions AS
SELECT pv.*
FROM payee_versions pv
WHERE pv.id = (
  SELECT current_pv.id
  FROM payee_versions current_pv
  WHERE current_pv.payee_id = pv.payee_id
    AND current_pv.effective_from <= date('now')
  ORDER BY current_pv.effective_from DESC, current_pv.version_seq DESC
  LIMIT 1
);

CREATE TABLE IF NOT EXISTS transactions (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  correction_of_transaction_id INTEGER REFERENCES transactions(id) ON DELETE RESTRICT,
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_request_id TEXT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  CHECK (correction_of_transaction_id IS NULL OR correction_of_transaction_id <> id)
);

CREATE INDEX IF NOT EXISTS transactions_book_idx
  ON transactions (book_id, id);

CREATE INDEX IF NOT EXISTS transactions_correction_idx
  ON transactions (correction_of_transaction_id)
  WHERE correction_of_transaction_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS transaction_versions (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE RESTRICT,
  version_seq INTEGER NOT NULL CHECK (version_seq > 0),
  supersedes_version_id INTEGER REFERENCES transaction_versions(id) ON DELETE RESTRICT,
  status TEXT NOT NULL CHECK (status IN ('draft', 'posted', 'voided')),
  transaction_kind TEXT NOT NULL CHECK (transaction_kind IN ('ordinary', 'transfer', 'investment', 'opening_balance', 'adjustment')),
  transaction_date TEXT NOT NULL CHECK (transaction_date GLOB '????-??-??'),
  payee_id INTEGER REFERENCES payees(id) ON DELETE RESTRICT,
  payee_name TEXT,
  description TEXT NOT NULL DEFAULT '',
  external_ref_hint TEXT,
  note_markdown TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  recorded_at TEXT NOT NULL,
  changed_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  change_reason TEXT NOT NULL,
  change_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  UNIQUE (transaction_id, version_seq)
);

CREATE INDEX IF NOT EXISTS transaction_versions_transaction_seq_idx
  ON transaction_versions (transaction_id, version_seq DESC);

CREATE INDEX IF NOT EXISTS transaction_versions_book_date_idx
  ON transaction_versions (book_id, transaction_date DESC, id DESC);

CREATE INDEX IF NOT EXISTS transaction_versions_payee_idx
  ON transaction_versions (payee_id)
  WHERE payee_id IS NOT NULL;

CREATE VIEW IF NOT EXISTS current_transaction_versions AS
SELECT tv.*
FROM transaction_versions tv
WHERE tv.id = (
  SELECT current_tv.id
  FROM transaction_versions current_tv
  WHERE current_tv.transaction_id = tv.transaction_id
  ORDER BY current_tv.version_seq DESC, current_tv.id DESC
  LIMIT 1
);

CREATE VIRTUAL TABLE IF NOT EXISTS transaction_search
USING fts5(
  transaction_version_id UNINDEXED,
  transaction_id UNINDEXED,
  payee_name,
  description,
  external_ref_hint,
  note_markdown
);

CREATE TABLE IF NOT EXISTS journal_entries (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  transaction_version_id INTEGER NOT NULL REFERENCES transaction_versions(id) ON DELETE RESTRICT,
  entry_seq INTEGER NOT NULL CHECK (entry_seq > 0),
  entry_date TEXT NOT NULL CHECK (entry_date GLOB '????-??-??'),
  entry_kind TEXT NOT NULL CHECK (entry_kind IN ('ordinary', 'transfer_leg', 'exchange', 'investment', 'opening_balance', 'adjustment')),
  memo TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  UNIQUE (transaction_version_id, entry_seq)
);

CREATE INDEX IF NOT EXISTS journal_entries_version_idx
  ON journal_entries (transaction_version_id, entry_seq);

CREATE INDEX IF NOT EXISTS journal_entries_book_date_idx
  ON journal_entries (book_id, entry_date DESC, id DESC);

CREATE TABLE IF NOT EXISTS posting_lines (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE RESTRICT,
  line_key TEXT NOT NULL CHECK (length(trim(line_key)) > 0 AND line_key = trim(line_key)),
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_request_id TEXT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  UNIQUE (book_id, transaction_id, line_key)
);

CREATE INDEX IF NOT EXISTS posting_lines_transaction_idx
  ON posting_lines (transaction_id);

CREATE TABLE IF NOT EXISTS posting_versions (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  transaction_version_id INTEGER NOT NULL REFERENCES transaction_versions(id) ON DELETE RESTRICT,
  journal_entry_id INTEGER NOT NULL REFERENCES journal_entries(id) ON DELETE RESTRICT,
  posting_line_id INTEGER NOT NULL REFERENCES posting_lines(id) ON DELETE RESTRICT,
  line_seq INTEGER NOT NULL CHECK (line_seq > 0),
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  quantity_value INTEGER NOT NULL,
  quantity_scale INTEGER NOT NULL CHECK (quantity_scale BETWEEN 0 AND 12),
  commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  memo TEXT NOT NULL DEFAULT '',
  reconciliation_status TEXT NOT NULL CHECK (reconciliation_status IN ('uncleared', 'cleared', 'reconciled')),
  cleared_on TEXT CHECK (cleared_on IS NULL OR cleared_on GLOB '????-??-??'),
  metadata_json TEXT NOT NULL DEFAULT '{}',
  UNIQUE (journal_entry_id, line_seq)
);

CREATE INDEX IF NOT EXISTS posting_versions_version_idx
  ON posting_versions (transaction_version_id);

CREATE INDEX IF NOT EXISTS posting_versions_account_idx
  ON posting_versions (account_id, id);

CREATE INDEX IF NOT EXISTS posting_versions_journal_idx
  ON posting_versions (journal_entry_id, line_seq);

CREATE TABLE IF NOT EXISTS transaction_tags (
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE RESTRICT,
  tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE RESTRICT,
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  PRIMARY KEY (transaction_id, tag_id)
);

CREATE INDEX IF NOT EXISTS transaction_tags_tag_idx
  ON transaction_tags (tag_id);

CREATE TABLE IF NOT EXISTS posting_tags (
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  posting_line_id INTEGER NOT NULL REFERENCES posting_lines(id) ON DELETE RESTRICT,
  tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE RESTRICT,
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  PRIMARY KEY (posting_line_id, tag_id)
);

CREATE INDEX IF NOT EXISTS posting_tags_tag_idx
  ON posting_tags (tag_id);

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
WHEN NOT (
  OLD.account_class IN ('income', 'expense')
  AND OLD.account_kind = OLD.account_class
  AND json_extract(OLD.metadata_json, '$.category.type') = OLD.account_class
  AND json_extract(OLD.metadata_json, '$.category.is_builtin') = 0
)
BEGIN
  SELECT RAISE(ABORT, 'account_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS payee_versions_no_update
BEFORE UPDATE ON payee_versions
BEGIN
  SELECT RAISE(ABORT, 'payee_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS payee_versions_no_delete
BEFORE DELETE ON payee_versions
BEGIN
  SELECT RAISE(ABORT, 'payee_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS transaction_versions_no_update
BEFORE UPDATE ON transaction_versions
BEGIN
  SELECT RAISE(ABORT, 'transaction_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS transaction_versions_no_delete
BEFORE DELETE ON transaction_versions
WHEN NOT (
  OLD.status = 'draft'
  AND NOT EXISTS (
    SELECT 1
    FROM transaction_versions tv
    WHERE tv.transaction_id = OLD.transaction_id
      AND tv.status IN ('posted', 'voided')
  )
)
BEGIN
  SELECT RAISE(ABORT, 'transaction_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS journal_entries_no_update
BEFORE UPDATE ON journal_entries
BEGIN
  SELECT RAISE(ABORT, 'journal_entries rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS journal_entries_no_delete
BEFORE DELETE ON journal_entries
WHEN NOT EXISTS (
  SELECT 1
  FROM transaction_versions tv
  WHERE tv.id = OLD.transaction_version_id
    AND tv.status = 'draft'
    AND NOT EXISTS (
      SELECT 1
      FROM transaction_versions any_tv
      WHERE any_tv.transaction_id = tv.transaction_id
        AND any_tv.status IN ('posted', 'voided')
    )
)
BEGIN
  SELECT RAISE(ABORT, 'journal_entries rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS posting_versions_no_update
BEFORE UPDATE ON posting_versions
BEGIN
  SELECT RAISE(ABORT, 'posting_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS posting_versions_no_delete
BEFORE DELETE ON posting_versions
WHEN NOT EXISTS (
  SELECT 1
  FROM transaction_versions tv
  WHERE tv.id = OLD.transaction_version_id
    AND tv.status = 'draft'
    AND NOT EXISTS (
      SELECT 1
      FROM transaction_versions any_tv
      WHERE any_tv.transaction_id = tv.transaction_id
        AND any_tv.status IN ('posted', 'voided')
    )
)
BEGIN
  SELECT RAISE(ABORT, 'posting_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS transaction_versions_same_book
BEFORE INSERT ON transaction_versions
WHEN NOT EXISTS (
  SELECT 1
  FROM transactions t
  WHERE t.id = NEW.transaction_id
    AND t.book_id = NEW.book_id
)
BEGIN
  SELECT RAISE(ABORT, 'transaction version book must match transaction book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS transaction_versions_supersedes_same_transaction
BEFORE INSERT ON transaction_versions
WHEN NEW.supersedes_version_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM transaction_versions superseded
    WHERE superseded.id = NEW.supersedes_version_id
      AND superseded.transaction_id = NEW.transaction_id
  )
BEGIN
  SELECT RAISE(ABORT, 'superseded transaction version must belong to the same transaction');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS transaction_versions_no_supersede_reconciled
BEFORE INSERT ON transaction_versions
WHEN NEW.supersedes_version_id IS NOT NULL
  AND EXISTS (
    SELECT 1
    FROM posting_versions pv
    WHERE pv.transaction_version_id = NEW.supersedes_version_id
      AND pv.reconciliation_status = 'reconciled'
  )
BEGIN
  SELECT RAISE(ABORT, 'reconciled postings require a corrective transaction');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS transaction_search_insert
AFTER INSERT ON transaction_versions
BEGIN
  INSERT INTO transaction_search (
    transaction_version_id,
    transaction_id,
    payee_name,
    description,
    external_ref_hint,
    note_markdown
  )
  VALUES (
    NEW.id,
    NEW.transaction_id,
    COALESCE(NEW.payee_name, ''),
    COALESCE(NEW.description, ''),
    COALESCE(NEW.external_ref_hint, ''),
    COALESCE(NEW.note_markdown, '')
  );
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS transactions_correction_same_book
BEFORE INSERT ON transactions
WHEN NEW.correction_of_transaction_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM transactions target
    WHERE target.id = NEW.correction_of_transaction_id
      AND target.book_id = NEW.book_id
  )
BEGIN
  SELECT RAISE(ABORT, 'correction target must belong to the same book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS journal_entries_same_book
BEFORE INSERT ON journal_entries
WHEN NOT EXISTS (
  SELECT 1
  FROM transaction_versions tv
  WHERE tv.id = NEW.transaction_version_id
    AND tv.book_id = NEW.book_id
)
BEGIN
  SELECT RAISE(ABORT, 'journal entry book must match transaction version book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS posting_lines_same_book
BEFORE INSERT ON posting_lines
WHEN NOT EXISTS (
  SELECT 1
  FROM transactions t
  WHERE t.id = NEW.transaction_id
    AND t.book_id = NEW.book_id
)
BEGIN
  SELECT RAISE(ABORT, 'posting line book must match transaction book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS posting_versions_same_book_and_lineage
BEFORE INSERT ON posting_versions
WHEN NOT EXISTS (
  SELECT 1
  FROM transaction_versions tv
  JOIN journal_entries je ON je.id = NEW.journal_entry_id
  JOIN posting_lines pl ON pl.id = NEW.posting_line_id
  JOIN accounts a ON a.id = NEW.account_id
  JOIN commodities c ON c.id = NEW.commodity_id
  WHERE tv.id = NEW.transaction_version_id
    AND tv.book_id = NEW.book_id
    AND je.transaction_version_id = tv.id
    AND je.book_id = NEW.book_id
    AND pl.book_id = NEW.book_id
    AND pl.transaction_id = tv.transaction_id
    AND a.book_id = NEW.book_id
    AND c.book_id = NEW.book_id
)
BEGIN
  SELECT RAISE(ABORT, 'posting version references must belong to one transaction book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS posting_versions_account_version_valid
BEFORE INSERT ON posting_versions
WHEN NOT EXISTS (
  SELECT 1
  FROM journal_entries je
  JOIN account_versions av ON av.account_id = NEW.account_id
  WHERE je.id = NEW.journal_entry_id
    AND av.id = (
      SELECT asof_av.id
      FROM account_versions asof_av
      WHERE asof_av.account_id = NEW.account_id
        AND asof_av.effective_from <= je.entry_date
      ORDER BY asof_av.effective_from DESC, asof_av.version_seq DESC
      LIMIT 1
    )
    AND av.status = 'active'
    AND av.allows_postings = 1
    AND je.entry_date >= av.opened_on
    AND (av.closed_on IS NULL OR je.entry_date <= av.closed_on)
    AND (av.default_commodity_id IS NULL OR av.default_commodity_id = NEW.commodity_id)
    AND (av.quantity_scale_override IS NULL OR NEW.quantity_scale <= av.quantity_scale_override)
)
BEGIN
  SELECT RAISE(ABORT, 'posting account is not eligible on entry date');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS posting_versions_commodity_scale_valid
BEFORE INSERT ON posting_versions
WHEN NOT EXISTS (
  SELECT 1
  FROM journal_entries je
  JOIN commodity_versions cv ON cv.commodity_id = NEW.commodity_id
  WHERE je.id = NEW.journal_entry_id
    AND cv.id = (
      SELECT asof_cv.id
      FROM commodity_versions asof_cv
      WHERE asof_cv.commodity_id = NEW.commodity_id
        AND asof_cv.effective_from <= je.entry_date
      ORDER BY asof_cv.effective_from DESC, asof_cv.version_seq DESC
      LIMIT 1
    )
    AND cv.status = 'active'
    AND NEW.quantity_scale <= cv.max_quantity_scale
)
BEGIN
  SELECT RAISE(ABORT, 'posting commodity scale is invalid');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS transaction_tags_same_book_and_active_tag
BEFORE INSERT ON transaction_tags
WHEN NOT EXISTS (
  SELECT 1
  FROM transactions t
  JOIN tags tag ON tag.id = NEW.tag_id
  WHERE t.id = NEW.transaction_id
    AND t.book_id = NEW.book_id
    AND tag.book_id = NEW.book_id
    AND tag.status = 'active'
)
BEGIN
  SELECT RAISE(ABORT, 'transaction tag must reference an active tag in the same book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS posting_tags_same_book_and_active_tag
BEFORE INSERT ON posting_tags
WHEN NOT EXISTS (
  SELECT 1
  FROM posting_lines pl
  JOIN tags tag ON tag.id = NEW.tag_id
  WHERE pl.id = NEW.posting_line_id
    AND pl.book_id = NEW.book_id
    AND tag.book_id = NEW.book_id
    AND tag.status = 'active'
)
BEGIN
  SELECT RAISE(ABORT, 'posting tag must reference an active tag in the same book');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS posting_tags_same_book_and_active_tag;
DROP TRIGGER IF EXISTS transaction_tags_same_book_and_active_tag;
DROP TRIGGER IF EXISTS posting_versions_commodity_scale_valid;
DROP TRIGGER IF EXISTS posting_versions_account_version_valid;
DROP TRIGGER IF EXISTS posting_versions_same_book_and_lineage;
DROP TRIGGER IF EXISTS posting_lines_same_book;
DROP TRIGGER IF EXISTS journal_entries_same_book;
DROP TRIGGER IF EXISTS transactions_correction_same_book;
DROP TRIGGER IF EXISTS transaction_search_insert;
DROP TRIGGER IF EXISTS transaction_versions_no_supersede_reconciled;
DROP TRIGGER IF EXISTS transaction_versions_supersedes_same_transaction;
DROP TRIGGER IF EXISTS transaction_versions_same_book;
DROP TRIGGER IF EXISTS transaction_versions_no_delete;
DROP TRIGGER IF EXISTS transaction_versions_no_update;
DROP TRIGGER IF EXISTS journal_entries_no_delete;
DROP TRIGGER IF EXISTS journal_entries_no_update;
DROP TRIGGER IF EXISTS posting_versions_no_delete;
DROP TRIGGER IF EXISTS posting_versions_no_update;
DROP TRIGGER IF EXISTS payee_versions_no_delete;
DROP TRIGGER IF EXISTS payee_versions_no_update;
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
DROP INDEX IF EXISTS posting_tags_tag_idx;
DROP TABLE IF EXISTS posting_tags;
DROP INDEX IF EXISTS transaction_tags_tag_idx;
DROP TABLE IF EXISTS transaction_tags;
DROP INDEX IF EXISTS posting_versions_journal_idx;
DROP INDEX IF EXISTS posting_versions_account_idx;
DROP INDEX IF EXISTS posting_versions_version_idx;
DROP TABLE IF EXISTS posting_versions;
DROP INDEX IF EXISTS posting_lines_transaction_idx;
DROP TABLE IF EXISTS posting_lines;
DROP INDEX IF EXISTS journal_entries_book_date_idx;
DROP INDEX IF EXISTS journal_entries_version_idx;
DROP TABLE IF EXISTS journal_entries;
DROP TABLE IF EXISTS transaction_search;
DROP VIEW IF EXISTS current_transaction_versions;
DROP INDEX IF EXISTS transaction_versions_payee_idx;
DROP INDEX IF EXISTS transaction_versions_book_date_idx;
DROP INDEX IF EXISTS transaction_versions_transaction_seq_idx;
DROP TABLE IF EXISTS transaction_versions;
DROP INDEX IF EXISTS transactions_correction_idx;
DROP INDEX IF EXISTS transactions_book_idx;
DROP TABLE IF EXISTS transactions;
DROP VIEW IF EXISTS current_payee_versions;
DROP INDEX IF EXISTS payee_versions_name_idx;
DROP INDEX IF EXISTS payee_versions_payee_seq_idx;
DROP TABLE IF EXISTS payee_versions;
DROP INDEX IF EXISTS payees_book_idx;
DROP TABLE IF EXISTS payees;
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
