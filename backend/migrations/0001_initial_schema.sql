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
  standard_scale INTEGER NOT NULL CHECK (standard_scale BETWEEN 0 AND 24),
  max_quantity_scale INTEGER NOT NULL CHECK (max_quantity_scale BETWEEN 0 AND 24 AND max_quantity_scale >= standard_scale),
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
  ('other_money_account', 'asset', 'bank_account', 1, 1, 'account_kind_other_money_account', 150),
  ('brokerage', 'asset', 'investment_container', 1, 1, 'account_kind_brokerage', 200),
  ('brokerage_cash', 'asset', 'investment_cash', 1, 1, 'account_kind_brokerage_cash', 210),
  ('security_holding', 'asset', 'security_holding', 1, 1, 'account_kind_security_holding', 220),
  ('fund_holding', 'asset', 'security_holding', 1, 1, 'account_kind_fund_holding', 225),
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
  quantity_scale_override INTEGER CHECK (quantity_scale_override IS NULL OR quantity_scale_override BETWEEN 0 AND 24),
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
  needs_review INTEGER NOT NULL DEFAULT 0 CHECK (needs_review IN (0, 1)),
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

CREATE INDEX IF NOT EXISTS transaction_versions_needs_review_idx
  ON transaction_versions (book_id, transaction_date DESC, id DESC)
  WHERE needs_review = 1;

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
  quantity_value TEXT NOT NULL DEFAULT '0' CHECK (length(quantity_value) BETWEEN 1 AND 39),
  quantity_scale INTEGER NOT NULL DEFAULT 0 CHECK (quantity_scale BETWEEN 0 AND 24),
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

CREATE TABLE IF NOT EXISTS reconciliation_sessions (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  status TEXT NOT NULL CHECK (status IN ('open', 'finished', 'voided')),
  source_kind TEXT NOT NULL CHECK (source_kind IN ('statement', 'online_balance', 'manual_cash_count', 'asset_valuation', 'other')),
  statement_date TEXT NOT NULL CHECK (statement_date GLOB '????-??-??'),
  statement_balance_value TEXT NOT NULL DEFAULT '0' CHECK (length(statement_balance_value) BETWEEN 1 AND 39),
  statement_balance_scale INTEGER NOT NULL DEFAULT 0 CHECK (statement_balance_scale BETWEEN 0 AND 24),
  starting_checkpoint_id INTEGER REFERENCES reconciliation_checkpoints(id) ON DELETE RESTRICT,
  starting_balance_value TEXT NOT NULL DEFAULT '0' CHECK (length(starting_balance_value) BETWEEN 1 AND 39),
  starting_balance_scale INTEGER NOT NULL DEFAULT 0 CHECK (starting_balance_scale BETWEEN 0 AND 24),
  selected_balance_value TEXT NOT NULL DEFAULT '0' CHECK (length(selected_balance_value) BETWEEN 1 AND 39),
  selected_balance_scale INTEGER NOT NULL DEFAULT 0 CHECK (selected_balance_scale BETWEEN 0 AND 24),
  difference_value TEXT NOT NULL DEFAULT '0' CHECK (length(difference_value) BETWEEN 1 AND 39),
  difference_scale INTEGER NOT NULL DEFAULT 0 CHECK (difference_scale BETWEEN 0 AND 24),
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  finished_at TEXT,
  voided_at TEXT,
  change_reason TEXT NOT NULL DEFAULT '',
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS reconciliation_sessions_account_idx
  ON reconciliation_sessions (book_id, account_id, commodity_id, status, statement_date DESC, id DESC);

CREATE TABLE IF NOT EXISTS reconciliation_session_postings (
  session_id INTEGER NOT NULL REFERENCES reconciliation_sessions(id) ON DELETE RESTRICT,
  posting_version_id INTEGER NOT NULL REFERENCES posting_versions(id) ON DELETE RESTRICT,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE RESTRICT,
  transaction_version_id INTEGER NOT NULL REFERENCES transaction_versions(id) ON DELETE RESTRICT,
  posting_line_id INTEGER NOT NULL REFERENCES posting_lines(id) ON DELETE RESTRICT,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  entry_date TEXT NOT NULL CHECK (entry_date GLOB '????-??-??'),
  quantity_value TEXT NOT NULL DEFAULT '0' CHECK (length(quantity_value) BETWEEN 1 AND 39),
  quantity_scale INTEGER NOT NULL DEFAULT 0 CHECK (quantity_scale BETWEEN 0 AND 24),
  payee_name TEXT,
  description TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  PRIMARY KEY (session_id, posting_version_id)
);

CREATE INDEX IF NOT EXISTS reconciliation_session_postings_posting_idx
  ON reconciliation_session_postings (posting_version_id);

CREATE TABLE IF NOT EXISTS reconciliation_checkpoints (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  session_id INTEGER REFERENCES reconciliation_sessions(id) ON DELETE RESTRICT,
  previous_checkpoint_id INTEGER REFERENCES reconciliation_checkpoints(id) ON DELETE RESTRICT,
  status TEXT NOT NULL CHECK (status IN ('active', 'invalidated', 'voided')),
  statement_date TEXT NOT NULL CHECK (statement_date GLOB '????-??-??'),
  statement_balance_value TEXT NOT NULL DEFAULT '0' CHECK (length(statement_balance_value) BETWEEN 1 AND 39),
  statement_balance_scale INTEGER NOT NULL DEFAULT 0 CHECK (statement_balance_scale BETWEEN 0 AND 24),
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  invalidated_at TEXT,
  invalidated_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  invalidation_reason TEXT NOT NULL DEFAULT '',
  voided_at TEXT,
  voided_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  void_reason TEXT NOT NULL DEFAULT '',
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  invalidated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  voided_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS reconciliation_checkpoints_account_idx
  ON reconciliation_checkpoints (book_id, account_id, commodity_id, status, statement_date DESC, id DESC);

CREATE TABLE IF NOT EXISTS reconciliation_checkpoint_postings (
  checkpoint_id INTEGER NOT NULL REFERENCES reconciliation_checkpoints(id) ON DELETE RESTRICT,
  posting_version_id INTEGER NOT NULL REFERENCES posting_versions(id) ON DELETE RESTRICT,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  entry_date TEXT NOT NULL CHECK (entry_date GLOB '????-??-??'),
  quantity_value TEXT NOT NULL DEFAULT '0' CHECK (length(quantity_value) BETWEEN 1 AND 39),
  quantity_scale INTEGER NOT NULL DEFAULT 0 CHECK (quantity_scale BETWEEN 0 AND 24),
  PRIMARY KEY (checkpoint_id, posting_version_id)
);

CREATE INDEX IF NOT EXISTS reconciliation_checkpoint_postings_posting_idx
  ON reconciliation_checkpoint_postings (posting_version_id);

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

CREATE TABLE IF NOT EXISTS market_data_sources (
  id INTEGER PRIMARY KEY,
  code TEXT NOT NULL UNIQUE CHECK (
    length(code) BETWEEN 1 AND 64
    AND code = trim(code)
    AND code GLOB '[a-z]*'
    AND code NOT GLOB '*[^a-z0-9_-]*'
  ),
  name TEXT NOT NULL CHECK (length(trim(name)) > 0 AND name = trim(name)),
  kind TEXT NOT NULL CHECK (kind IN ('manual', 'provider', 'import')),
  provider_key TEXT,
  base_url TEXT,
  status TEXT NOT NULL CHECK (status IN ('active', 'disabled')),
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

INSERT OR IGNORE INTO market_data_sources (
  id, code, name, kind, status, metadata_json, created_at
) VALUES (
  1, 'manual', 'Manual', 'manual', 'active', '{}', '0001-01-01T00:00:00Z'
);

INSERT OR IGNORE INTO market_data_sources (
  code, name, kind, provider_key, base_url, status, metadata_json, created_at
) VALUES
  (
    'ecb_euro_reference_rates',
    'ECB euro foreign exchange reference rates',
    'provider',
    'ecb_euro_reference_rates',
    'https://www.ecb.europa.eu/stats/eurofxref/',
    'active',
    '{"capabilities":["fx_rates"],"quote_type":"official_fixing","requires_secret":false}',
    '0001-01-01T00:00:00Z'
  ),
  (
    'frankfurter',
    'Frankfurter',
    'provider',
    'frankfurter',
    'https://api.frankfurter.dev/v2/',
    'active',
    '{"capabilities":["fx_rates","fx_currencies"],"quote_type":"official_fixing","requires_secret":false,"supports_provider_filter":true}',
    '0001-01-01T00:00:00Z'
  ),
  (
    'exchangerate_api_open_access',
    'ExchangeRate-API Open Access',
    'provider',
    'exchangerate_api_open_access',
    'https://open.er-api.com/v6/',
    'active',
    '{"capabilities":["fx_rates"],"quote_type":"official_fixing","requires_secret":false,"attribution_required":true}',
    '0001-01-01T00:00:00Z'
  ),
  (
    'open_exchange_rates_free',
    'Open Exchange Rates Free',
    'provider',
    'open_exchange_rates_free',
    'https://openexchangerates.org/api/',
    'disabled',
    '{"capabilities":["fx_rates"],"quote_type":"official_fixing","requires_secret":true,"secret_ref":"OPEN_EXCHANGE_RATES_APP_ID"}',
    '0001-01-01T00:00:00Z'
  );

CREATE TABLE IF NOT EXISTS investment_instruments (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_request_id TEXT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  UNIQUE (book_id, commodity_id)
);

CREATE INDEX IF NOT EXISTS investment_instruments_book_idx
  ON investment_instruments (book_id);

CREATE TABLE IF NOT EXISTS investment_instrument_versions (
  id INTEGER PRIMARY KEY,
  instrument_id INTEGER NOT NULL REFERENCES investment_instruments(id) ON DELETE RESTRICT,
  version_seq INTEGER NOT NULL CHECK (version_seq > 0),
  effective_from TEXT NOT NULL CHECK (effective_from GLOB '????-??-??'),
  recorded_at TEXT NOT NULL,
  changed_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  change_reason TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
  instrument_type TEXT NOT NULL CHECK (instrument_type IN ('stock', 'etf', 'fund', 'bond', 'option', 'future', 'other')),
  display_name TEXT NOT NULL CHECK (length(trim(display_name)) > 0 AND display_name = trim(display_name)),
  symbol TEXT CHECK (symbol IS NULL OR (length(trim(symbol)) > 0 AND symbol = trim(symbol))),
  exchange_code TEXT,
  mic TEXT,
  issuer TEXT,
  country_code TEXT,
  quote_commodity_id INTEGER REFERENCES commodities(id) ON DELETE RESTRICT,
  trading_commodity_id INTEGER REFERENCES commodities(id) ON DELETE RESTRICT,
  quantity_scale INTEGER NOT NULL CHECK (quantity_scale BETWEEN 0 AND 12),
  price_scale INTEGER NOT NULL CHECK (price_scale BETWEEN 0 AND 12),
  identifiers_json TEXT NOT NULL DEFAULT '{}',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  change_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  UNIQUE (instrument_id, version_seq)
);

CREATE INDEX IF NOT EXISTS investment_instrument_versions_instrument_seq_idx
  ON investment_instrument_versions (instrument_id, version_seq DESC);

CREATE INDEX IF NOT EXISTS investment_instrument_versions_symbol_idx
  ON investment_instrument_versions (symbol COLLATE NOCASE, exchange_code, mic);

CREATE VIEW IF NOT EXISTS current_investment_instrument_versions AS
SELECT iiv.*
FROM investment_instrument_versions iiv
WHERE iiv.id = (
  SELECT current_iiv.id
  FROM investment_instrument_versions current_iiv
  WHERE current_iiv.instrument_id = iiv.instrument_id
    AND current_iiv.effective_from <= date('now')
  ORDER BY current_iiv.effective_from DESC, current_iiv.version_seq DESC
  LIMIT 1
);

CREATE TABLE IF NOT EXISTS instrument_source_links (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  instrument_id INTEGER NOT NULL REFERENCES investment_instruments(id) ON DELETE RESTRICT,
  source_id INTEGER NOT NULL REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  provider_instrument_id TEXT,
  symbol TEXT NOT NULL CHECK (length(trim(symbol)) > 0 AND symbol = trim(symbol)),
  exchange_code TEXT,
  mic TEXT,
  confidence_bps INTEGER NOT NULL DEFAULT 10000 CHECK (confidence_bps BETWEEN 0 AND 10000),
  status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
  effective_from TEXT NOT NULL CHECK (effective_from GLOB '????-??-??'),
  effective_to TEXT CHECK (effective_to IS NULL OR effective_to GLOB '????-??-??'),
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  CHECK (effective_to IS NULL OR effective_to >= effective_from)
);

CREATE INDEX IF NOT EXISTS instrument_source_links_lookup_idx
  ON instrument_source_links (book_id, source_id, symbol COLLATE NOCASE, exchange_code, mic, status);

CREATE UNIQUE INDEX IF NOT EXISTS instrument_source_links_active_provider_idx
  ON instrument_source_links (book_id, source_id, provider_instrument_id)
  WHERE status = 'active' AND provider_instrument_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS instrument_search_cache (
  id INTEGER PRIMARY KEY,
  source_id INTEGER NOT NULL REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  query TEXT NOT NULL,
  provider_instrument_id TEXT,
  symbol TEXT NOT NULL,
  exchange_code TEXT,
  mic TEXT,
  display_name TEXT NOT NULL,
  instrument_type TEXT NOT NULL,
  quote_code TEXT,
  country_code TEXT,
  confidence_bps INTEGER NOT NULL DEFAULT 0 CHECK (confidence_bps BETWEEN 0 AND 10000),
  raw_json TEXT NOT NULL DEFAULT '{}',
  fetched_at TEXT NOT NULL,
  expires_at TEXT
);

CREATE INDEX IF NOT EXISTS instrument_search_cache_query_idx
  ON instrument_search_cache (source_id, query COLLATE NOCASE, fetched_at DESC);

CREATE TABLE IF NOT EXISTS pricing_policies (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT UNIQUE,
  base_commodity_id INTEGER REFERENCES commodities(id) ON DELETE RESTRICT,
  default_source_id INTEGER REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  refresh_enabled INTEGER NOT NULL DEFAULT 1 CHECK (refresh_enabled IN (0, 1)),
  refresh_hour_utc INTEGER NOT NULL DEFAULT 4 CHECK (refresh_hour_utc BETWEEN 0 AND 23),
  refresh_minute_utc INTEGER NOT NULL DEFAULT 0 CHECK (refresh_minute_utc BETWEEN 0 AND 59),
  refresh_hour_local INTEGER NOT NULL DEFAULT 4 CHECK (refresh_hour_local BETWEEN 0 AND 23),
  refresh_minute_local INTEGER NOT NULL DEFAULT 0 CHECK (refresh_minute_local BETWEEN 0 AND 59),
  max_backfill_days INTEGER NOT NULL DEFAULT 370 CHECK (max_backfill_days >= 1),
  staleness_max_days INTEGER NOT NULL DEFAULT 3 CHECK (staleness_max_days >= 1),
  triangulation_max_hops INTEGER NOT NULL DEFAULT 1 CHECK (triangulation_max_hops BETWEEN 0 AND 4),
  rounding_mode TEXT NOT NULL DEFAULT 'half_up' CHECK (rounding_mode IN ('half_up', 'half_even', 'down', 'up')),
  prefer_official_fx INTEGER NOT NULL DEFAULT 1 CHECK (prefer_official_fx IN (0, 1)),
  weekend_policy TEXT NOT NULL DEFAULT 'skip' CHECK (weekend_policy IN ('skip', 'fill_previous', 'download')),
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS pricing_source_assignments (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  base_commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  quote_commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  source_id INTEGER NOT NULL REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  priority INTEGER NOT NULL DEFAULT 100 CHECK (priority >= 0),
  status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
  effective_from TEXT NOT NULL CHECK (effective_from GLOB '????-??-??'),
  effective_to TEXT CHECK (effective_to IS NULL OR effective_to GLOB '????-??-??'),
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  CHECK (effective_to IS NULL OR effective_to >= effective_from)
);

CREATE INDEX IF NOT EXISTS pricing_source_assignments_lookup_idx
  ON pricing_source_assignments (book_id, base_commodity_id, quote_commodity_id, status, effective_from DESC, priority);

CREATE TABLE IF NOT EXISTS price_series (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  base_commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  quote_commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  source_id INTEGER REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  quote_type TEXT NOT NULL CHECK (quote_type IN ('close', 'adjusted_close', 'nav', 'official_fixing', 'spot_mid', 'bid', 'ask', 'manual', 'trade_implied', 'valuation_override')),
  adjustment_basis TEXT NOT NULL DEFAULT 'raw' CHECK (adjustment_basis IN ('raw', 'split_adjusted', 'dividend_adjusted', 'total_return', 'not_applicable')),
  market_code TEXT,
  provider_series_id TEXT,
  status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS price_series_lookup_idx
  ON price_series (book_id, base_commodity_id, quote_commodity_id, quote_type, adjustment_basis, status, source_id);

CREATE UNIQUE INDEX IF NOT EXISTS price_series_provider_idx
  ON price_series (book_id, source_id, provider_series_id)
  WHERE source_id IS NOT NULL AND provider_series_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS price_observations (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  series_id INTEGER NOT NULL REFERENCES price_series(id) ON DELETE RESTRICT,
  base_commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  quote_commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  quote_type TEXT NOT NULL CHECK (quote_type IN ('close', 'adjusted_close', 'nav', 'official_fixing', 'spot_mid', 'bid', 'ask', 'manual', 'trade_implied', 'valuation_override')),
  adjustment_basis TEXT NOT NULL DEFAULT 'raw' CHECK (adjustment_basis IN ('raw', 'split_adjusted', 'dividend_adjusted', 'total_return', 'not_applicable')),
  price_value INTEGER NOT NULL,
  price_scale INTEGER NOT NULL CHECK (price_scale BETWEEN 0 AND 18),
  base_quantity_value INTEGER NOT NULL DEFAULT 1 CHECK (base_quantity_value > 0),
  base_quantity_scale INTEGER NOT NULL DEFAULT 0 CHECK (base_quantity_scale BETWEEN 0 AND 18),
  valuation_date TEXT NOT NULL CHECK (valuation_date GLOB '????-??-??'),
  observed_at TEXT,
  source_published_at TEXT,
  source_id INTEGER REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  provider_observation_id TEXT,
  is_manual INTEGER NOT NULL DEFAULT 0 CHECK (is_manual IN (0, 1)),
  is_derived INTEGER NOT NULL DEFAULT 0 CHECK (is_derived IN (0, 1)),
  supersedes_observation_id INTEGER REFERENCES price_observations(id) ON DELETE RESTRICT,
  ingest_run_id INTEGER,
  derivation_json TEXT NOT NULL DEFAULT '{}',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  voided_at TEXT,
  voided_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  void_reason TEXT NOT NULL DEFAULT '',
  recorded_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  -- Voiding a price observation is a user operation, so it needs its own
  -- audit_events reference. created_audit_event_id records the insert; without
  -- this column a void's who/when/why would be an orphaned audit row (T-37).
  voided_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS price_observations_voided_idx
  ON price_observations (book_id, voided_at)
  WHERE voided_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS price_observations_lookup_idx
  ON price_observations (book_id, base_commodity_id, quote_commodity_id, quote_type, adjustment_basis, valuation_date DESC, recorded_at DESC, id DESC);

CREATE UNIQUE INDEX IF NOT EXISTS price_observations_provider_event_idx
  ON price_observations (source_id, provider_observation_id)
  WHERE provider_observation_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS market_data_ingest_runs (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  source_id INTEGER NOT NULL REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  trigger TEXT NOT NULL CHECK (trigger IN ('manual', 'scheduled', 'domain', 'recovery', 'provider_callback', 'import')),
  status TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed', 'partial')),
  started_at TEXT NOT NULL,
  finished_at TEXT,
  items_total INTEGER NOT NULL DEFAULT 0 CHECK (items_total >= 0),
  items_succeeded INTEGER NOT NULL DEFAULT 0 CHECK (items_succeeded >= 0),
  items_failed INTEGER NOT NULL DEFAULT 0 CHECK (items_failed >= 0),
  last_error TEXT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS market_data_ingest_runs_book_idx
  ON market_data_ingest_runs (book_id, started_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS market_data_ingest_items (
  id INTEGER PRIMARY KEY,
  run_id INTEGER NOT NULL REFERENCES market_data_ingest_runs(id) ON DELETE RESTRICT,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  item_kind TEXT NOT NULL CHECK (item_kind IN ('instrument', 'price', 'dividend', 'corporate_action')),
  status TEXT NOT NULL CHECK (status IN ('succeeded', 'failed', 'skipped')),
  provider_item_id TEXT,
  local_ref_table TEXT,
  local_ref_id INTEGER,
  error TEXT,
  raw_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS market_data_ingest_items_run_idx
  ON market_data_ingest_items (run_id, item_kind, status);

CREATE TABLE IF NOT EXISTS pricing_refresh_state (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  base_commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  quote_commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  source_id INTEGER NOT NULL REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  last_success_date TEXT CHECK (last_success_date IS NULL OR last_success_date GLOB '????-??-??'),
  last_attempt_at TEXT,
  last_error TEXT,
  updated_at TEXT NOT NULL,
  UNIQUE (book_id, base_commodity_id, quote_commodity_id, source_id)
);

CREATE TABLE IF NOT EXISTS cost_basis_profiles (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  name TEXT NOT NULL CHECK (length(trim(name)) > 0 AND name = trim(name)),
  method TEXT NOT NULL CHECK (method IN ('fifo', 'lifo', 'average_cost', 'specific_lot')),
  is_default INTEGER NOT NULL CHECK (is_default IN (0, 1)),
  status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
  description TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  UNIQUE (book_id, name COLLATE NOCASE)
);

CREATE UNIQUE INDEX IF NOT EXISTS cost_basis_profiles_default_idx
  ON cost_basis_profiles (book_id)
  WHERE is_default = 1 AND status = 'active';

CREATE TABLE IF NOT EXISTS dividend_defaults (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  commodity_id INTEGER REFERENCES commodities(id) ON DELETE RESTRICT,
  income_account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  withholding_account_id INTEGER REFERENCES accounts(id) ON DELETE RESTRICT,
  default_withholding_value INTEGER,
  default_withholding_scale INTEGER CHECK (default_withholding_scale IS NULL OR default_withholding_scale BETWEEN 0 AND 12),
  withholding_rate_bps INTEGER CHECK (withholding_rate_bps IS NULL OR withholding_rate_bps BETWEEN 0 AND 10000),
  tax_country_code TEXT,
  tax_treatment TEXT,
  status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
  effective_from TEXT NOT NULL CHECK (effective_from GLOB '????-??-??'),
  effective_to TEXT CHECK (effective_to IS NULL OR effective_to GLOB '????-??-??'),
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  CHECK (effective_to IS NULL OR effective_to >= effective_from)
);

CREATE INDEX IF NOT EXISTS dividend_defaults_lookup_idx
  ON dividend_defaults (book_id, commodity_id, status, effective_from DESC);

CREATE TABLE IF NOT EXISTS investment_lots (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  opened_on TEXT NOT NULL CHECK (opened_on GLOB '????-??-??'),
  source_transaction_id INTEGER REFERENCES transactions(id) ON DELETE RESTRICT,
  status TEXT NOT NULL CHECK (status IN ('open', 'closed')),
  quantity_value TEXT NOT NULL DEFAULT '0' CHECK (length(quantity_value) BETWEEN 1 AND 38),
  quantity_scale INTEGER NOT NULL DEFAULT 0 CHECK (quantity_scale BETWEEN 0 AND 24),
  remaining_quantity_value TEXT NOT NULL DEFAULT '0' CHECK (length(remaining_quantity_value) BETWEEN 1 AND 38),
  remaining_quantity_scale INTEGER NOT NULL DEFAULT 0 CHECK (remaining_quantity_scale BETWEEN 0 AND 24),
  cost_basis_value INTEGER NOT NULL,
  cost_basis_scale INTEGER NOT NULL CHECK (cost_basis_scale BETWEEN 0 AND 12),
  remaining_cost_basis_value INTEGER NOT NULL,
  remaining_cost_basis_scale INTEGER NOT NULL CHECK (remaining_cost_basis_scale BETWEEN 0 AND 12),
  cost_commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS investment_lots_position_idx
  ON investment_lots (book_id, account_id, commodity_id, status, opened_on, id);

CREATE TABLE IF NOT EXISTS investment_lot_events (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  lot_id INTEGER NOT NULL REFERENCES investment_lots(id) ON DELETE RESTRICT,
  event_kind TEXT NOT NULL CHECK (event_kind IN ('acquisition', 'disposal', 'split_adjustment', 'reinvested_dividend', 'manual_adjustment')),
  transaction_id INTEGER REFERENCES transactions(id) ON DELETE RESTRICT,
  event_date TEXT NOT NULL CHECK (event_date GLOB '????-??-??'),
  quantity_value TEXT NOT NULL DEFAULT '0' CHECK (length(quantity_value) BETWEEN 1 AND 39),
  quantity_scale INTEGER NOT NULL DEFAULT 0 CHECK (quantity_scale BETWEEN 0 AND 24),
  cost_basis_value INTEGER NOT NULL DEFAULT 0,
  cost_basis_scale INTEGER NOT NULL DEFAULT 0 CHECK (cost_basis_scale BETWEEN 0 AND 12),
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS investment_lot_events_lot_idx
  ON investment_lot_events (lot_id, event_date, id);

CREATE TABLE IF NOT EXISTS investment_provider_events (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  source_id INTEGER NOT NULL REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  provider_event_id TEXT,
  instrument_id INTEGER REFERENCES investment_instruments(id) ON DELETE RESTRICT,
  event_family TEXT NOT NULL CHECK (event_family IN ('dividend', 'distribution', 'split', 'merger', 'spin_off', 'ticker_change', 'delisting', 'cash_in_lieu', 'return_of_capital', 'corporate_action')),
  event_date TEXT NOT NULL CHECK (event_date GLOB '????-??-??'),
  status TEXT NOT NULL CHECK (status IN ('new', 'matched', 'ignored', 'superseded')),
  normalized_json TEXT NOT NULL DEFAULT '{}',
  raw_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS investment_provider_events_source_event_idx
  ON investment_provider_events (source_id, provider_event_id)
  WHERE provider_event_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS investment_provider_events_book_idx
  ON investment_provider_events (book_id, event_family, event_date DESC, id DESC);

CREATE TABLE IF NOT EXISTS investment_event_suggestions (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  provider_event_id INTEGER NOT NULL REFERENCES investment_provider_events(id) ON DELETE RESTRICT,
  instrument_id INTEGER REFERENCES investment_instruments(id) ON DELETE RESTRICT,
  confidence_bps INTEGER NOT NULL CHECK (confidence_bps BETWEEN 0 AND 10000),
  status TEXT NOT NULL CHECK (status IN ('suggested', 'accepted', 'ignored', 'auto_posted', 'failed', 'superseded')),
  proposed_transaction_json TEXT NOT NULL DEFAULT '{}',
  generated_transaction_id INTEGER REFERENCES transactions(id) ON DELETE RESTRICT,
  failure_reason TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS investment_event_suggestions_book_idx
  ON investment_event_suggestions (book_id, status, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS investment_automation_rules (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  source_id INTEGER REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  instrument_id INTEGER REFERENCES investment_instruments(id) ON DELETE RESTRICT,
  event_family TEXT NOT NULL CHECK (event_family IN ('dividend', 'distribution', 'split', 'merger', 'spin_off', 'ticker_change', 'delisting', 'cash_in_lieu', 'return_of_capital', 'corporate_action')),
  mode TEXT NOT NULL CHECK (mode IN ('suggest', 'auto_post')),
  confidence_threshold_bps INTEGER NOT NULL CHECK (confidence_threshold_bps BETWEEN 0 AND 10000),
  required_accounts_json TEXT NOT NULL DEFAULT '{}',
  status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
  effective_from TEXT NOT NULL CHECK (effective_from GLOB '????-??-??'),
  effective_to TEXT CHECK (effective_to IS NULL OR effective_to GLOB '????-??-??'),
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  CHECK (effective_to IS NULL OR effective_to >= effective_from)
);

CREATE INDEX IF NOT EXISTS investment_automation_rules_lookup_idx
  ON investment_automation_rules (book_id, event_family, status, source_id, instrument_id);

CREATE TABLE IF NOT EXISTS background_work_items (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  kind TEXT NOT NULL CHECK (length(trim(kind)) > 0 AND kind = trim(kind)),
  payload_version INTEGER NOT NULL DEFAULT 1 CHECK (payload_version > 0),
  payload_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(payload_json)),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed')),
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  available_at TEXT NOT NULL,
  lease_owner TEXT,
  lease_expires_at TEXT,
  last_error TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  completed_at TEXT,
  CHECK (
    (status = 'running' AND lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL)
    OR status != 'running'
  )
);

CREATE INDEX IF NOT EXISTS background_work_items_due_idx
  ON background_work_items (status, available_at, lease_expires_at, id);

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
CREATE TRIGGER IF NOT EXISTS commodity_versions_kind_scale_valid
BEFORE INSERT ON commodity_versions
WHEN NEW.standard_scale > 24
  OR NEW.max_quantity_scale > 24
  OR (
    (SELECT kind FROM commodities WHERE id = NEW.commodity_id) <> 'crypto'
    AND (NEW.standard_scale > 12 OR NEW.max_quantity_scale > 12)
  )
BEGIN
  SELECT RAISE(ABORT, 'commodity precision is invalid for commodity kind');
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
WHEN EXISTS (
  SELECT 1
  FROM account_versions av
  WHERE av.institution_id = OLD.institution_id
)
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
  (
    OLD.account_class IN ('income', 'expense')
    AND OLD.account_kind = OLD.account_class
    AND json_extract(OLD.metadata_json, '$.category.type') = OLD.account_class
    AND json_extract(OLD.metadata_json, '$.category.is_builtin') = 0
  )
  OR (
    NOT EXISTS (
      SELECT 1
      FROM accounts a
      WHERE a.id = OLD.account_id
        AND a.system_role IS NOT NULL
    )
    AND NOT EXISTS (
      SELECT 1
      FROM posting_versions pv
      WHERE pv.account_id = OLD.account_id
    )
    AND NOT EXISTS (
      SELECT 1
      FROM account_versions child
      WHERE child.parent_account_id = OLD.account_id
    )
  )
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
      SELECT asof_av.id FROM account_versions asof_av
      WHERE asof_av.account_id = NEW.account_id AND asof_av.effective_from <= je.entry_date
      ORDER BY asof_av.effective_from DESC, asof_av.version_seq DESC LIMIT 1
    )
    AND av.status = 'active' AND av.allows_postings = 1
    AND (av.opened_on IS NULL OR je.entry_date >= av.opened_on)
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
  JOIN commodities c ON c.id = cv.commodity_id
  WHERE je.id = NEW.journal_entry_id
    AND cv.id = (
      SELECT asof_cv.id FROM commodity_versions asof_cv
      WHERE asof_cv.commodity_id = NEW.commodity_id AND asof_cv.effective_from <= je.entry_date
      ORDER BY asof_cv.effective_from DESC, asof_cv.version_seq DESC LIMIT 1
    )
    AND cv.status = 'active'
    AND NEW.quantity_scale <= cv.max_quantity_scale
    AND NEW.quantity_scale <= CASE WHEN c.kind = 'crypto' THEN 24 ELSE 12 END
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

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS investment_instrument_versions_no_update
BEFORE UPDATE ON investment_instrument_versions
BEGIN
  SELECT RAISE(ABORT, 'investment_instrument_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS investment_instrument_versions_no_delete
BEFORE DELETE ON investment_instrument_versions
BEGIN
  SELECT RAISE(ABORT, 'investment_instrument_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS investment_instruments_commodity_must_be_security
BEFORE INSERT ON investment_instruments
WHEN NOT EXISTS (
  SELECT 1
  FROM commodities c
  WHERE c.id = NEW.commodity_id
    AND c.book_id = NEW.book_id
    AND c.kind = 'security'
)
BEGIN
  SELECT RAISE(ABORT, 'investment instrument commodity must be a security in the same book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS investment_lots_same_book
BEFORE INSERT ON investment_lots
WHEN NOT EXISTS (
  SELECT 1
  FROM accounts a
  JOIN commodities security ON security.id = NEW.commodity_id
  JOIN commodities cost ON cost.id = NEW.cost_commodity_id
  WHERE a.id = NEW.account_id
    AND a.book_id = NEW.book_id
    AND security.book_id = NEW.book_id
    AND security.kind = 'security'
    AND cost.book_id = NEW.book_id
)
BEGIN
  SELECT RAISE(ABORT, 'investment lot references must belong to one book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS investment_lot_events_same_book
BEFORE INSERT ON investment_lot_events
WHEN NOT EXISTS (
  SELECT 1
  FROM investment_lots lot
  WHERE lot.id = NEW.lot_id
    AND lot.book_id = NEW.book_id
)
BEGIN
  SELECT RAISE(ABORT, 'investment lot event must reference a lot in the same book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS investment_lots_values_valid_insert
BEFORE INSERT ON investment_lots
WHEN NEW.quantity_value = '0' OR substr(NEW.quantity_value, 1, 1) = '-'
  OR substr(NEW.remaining_quantity_value, 1, 1) = '-'
  OR NEW.cost_basis_value < 0 OR NEW.remaining_cost_basis_value < 0
BEGIN
  SELECT RAISE(ABORT, 'investment lot values are invalid');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS investment_lots_values_valid_update
BEFORE UPDATE ON investment_lots
WHEN NEW.quantity_value = '0' OR substr(NEW.quantity_value, 1, 1) = '-'
  OR substr(NEW.remaining_quantity_value, 1, 1) = '-'
  OR NEW.cost_basis_value < 0 OR NEW.remaining_cost_basis_value < 0
BEGIN
  SELECT RAISE(ABORT, 'investment lot values are invalid');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS pricing_source_assignments_same_book
BEFORE INSERT ON pricing_source_assignments
WHEN NOT EXISTS (
  SELECT 1
  FROM commodities b
  JOIN commodities q ON q.id = NEW.quote_commodity_id
  WHERE b.id = NEW.base_commodity_id
    AND b.book_id = NEW.book_id
    AND q.book_id = NEW.book_id
)
BEGIN
  SELECT RAISE(ABORT, 'pricing source assignment commodities must belong to one book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS price_series_same_book
BEFORE INSERT ON price_series
WHEN NOT EXISTS (
  SELECT 1
  FROM commodities b
  JOIN commodities q ON q.id = NEW.quote_commodity_id
  WHERE b.id = NEW.base_commodity_id
    AND b.book_id = NEW.book_id
    AND q.book_id = NEW.book_id
)
BEGIN
  SELECT RAISE(ABORT, 'price series commodities must belong to one book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS price_observations_same_book
BEFORE INSERT ON price_observations
WHEN NOT EXISTS (
  SELECT 1
  FROM price_series ps
  JOIN commodities c ON c.id = NEW.base_commodity_id
  JOIN commodities q ON q.id = NEW.quote_commodity_id
  WHERE ps.id = NEW.series_id
    AND ps.book_id = NEW.book_id
    AND ps.base_commodity_id = NEW.base_commodity_id
    AND ps.quote_commodity_id = NEW.quote_commodity_id
    AND c.book_id = NEW.book_id
    AND q.book_id = NEW.book_id
)
BEGIN
  SELECT RAISE(ABORT, 'price observation series and commodities must belong to one book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS price_observations_positive_insert
BEFORE INSERT ON price_observations
WHEN NEW.price_value <= 0
BEGIN
  SELECT RAISE(ABORT, 'price observation value must be positive');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS price_observations_positive_update
BEFORE UPDATE ON price_observations
WHEN NEW.price_value <= 0
BEGIN
  SELECT RAISE(ABORT, 'price observation value must be positive');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER fx_work_after_account_version_insert
AFTER INSERT ON account_versions
WHEN NEW.status = 'active'
  AND NEW.default_commodity_id IS NOT NULL
  AND EXISTS (
    SELECT 1 FROM commodities c
    WHERE c.id = NEW.default_commodity_id AND c.kind = 'currency'
  )
  AND NEW.default_commodity_id != COALESCE(
    (SELECT pp.base_commodity_id
     FROM accounts a JOIN pricing_policies pp ON pp.book_id = a.book_id
     WHERE a.id = NEW.account_id),
    (SELECT b.default_currency_commodity_id
     FROM accounts a JOIN books b ON b.id = a.book_id
     WHERE a.id = NEW.account_id),
    0
  )
  AND NOT EXISTS (
    SELECT 1 FROM account_versions prior
    WHERE prior.account_id = NEW.account_id AND prior.id != NEW.id
      AND prior.status = 'active'
      AND prior.default_commodity_id = NEW.default_commodity_id
  )
  AND NOT EXISTS (
    SELECT 1
    FROM current_account_versions other
    JOIN accounts other_account ON other_account.id = other.account_id
    JOIN accounts new_account ON new_account.id = NEW.account_id
    WHERE other_account.book_id = new_account.book_id
      AND other.account_id != NEW.account_id
      AND other.status = 'active'
      AND other.default_commodity_id = NEW.default_commodity_id
  )
BEGIN
  INSERT INTO background_work_items (
    book_id, kind, payload_json, available_at, created_at, updated_at
  )
  SELECT a.book_id, 'pricing.fx_coverage',
    json_object('reason', 'currency_activated', 'start_date', NEW.opened_on,
      'currency_id', NEW.default_commodity_id),
    NEW.recorded_at, NEW.recorded_at, NEW.recorded_at
  FROM accounts a WHERE a.id = NEW.account_id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER fx_work_after_posting_version_insert
AFTER INSERT ON posting_versions
WHEN EXISTS (
  SELECT 1 FROM transaction_versions tv
  WHERE tv.id = NEW.transaction_version_id AND tv.status = 'posted'
)
  AND EXISTS (SELECT 1 FROM commodities c WHERE c.id = NEW.commodity_id AND c.kind = 'currency')
  AND NEW.commodity_id != COALESCE(
    (SELECT base_commodity_id FROM pricing_policies WHERE book_id = NEW.book_id),
    (SELECT default_currency_commodity_id FROM books WHERE id = NEW.book_id),
    0
  )
  AND NOT EXISTS (
    SELECT 1 FROM background_work_items bw
    WHERE bw.book_id = NEW.book_id
      AND bw.kind = 'pricing.fx_coverage'
      AND bw.status IN ('pending', 'running')
      AND json_extract(bw.payload_json, '$.currency_id') = NEW.commodity_id
      AND json_extract(bw.payload_json, '$.start_date') <= (SELECT entry_date FROM journal_entries WHERE id = NEW.journal_entry_id)
  )
BEGIN
  INSERT INTO background_work_items (
    book_id, kind, payload_json, available_at, created_at, updated_at
  ) VALUES (
    NEW.book_id, 'pricing.fx_coverage',
    json_object('reason', 'transaction_entered',
      'start_date', (SELECT entry_date FROM journal_entries WHERE id = NEW.journal_entry_id),
      'currency_id', NEW.commodity_id),
    strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
    strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
    strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
  );
END;
-- +goose StatementEnd

-- Beta baseline extensions. This repository intentionally has no pre-beta
-- databases to upgrade, so the former 0002–0011 migrations are folded into
-- this fresh-install schema in their original execution order. Keeping the
-- statements (rather than hand-reconstructing their final DDL) preserves the
-- exact defaults, indexes, and trigger replacement semantics that the prior
-- chain produced.

ALTER TABLE transactions ADD COLUMN deleted_at TEXT;
ALTER TABLE transactions ADD COLUMN deleted_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT;
ALTER TABLE transactions ADD COLUMN deleted_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT;
ALTER TABLE transactions ADD COLUMN delete_reason TEXT;

CREATE INDEX transactions_deleted_idx
  ON transactions (book_id, deleted_at, id)
  WHERE deleted_at IS NOT NULL;

CREATE TABLE transaction_deletion_events (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE RESTRICT,
  action TEXT NOT NULL CHECK (action IN ('soft_delete', 'restore')),
  occurred_at TEXT NOT NULL,
  actor_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  audit_event_id INTEGER NOT NULL REFERENCES audit_events(id) ON DELETE RESTRICT,
  reason TEXT NOT NULL
);

CREATE INDEX transaction_deletion_events_transaction_idx
  ON transaction_deletion_events (transaction_id, id);

-- +goose StatementBegin
CREATE TRIGGER transaction_deletion_events_no_update
BEFORE UPDATE ON transaction_deletion_events
BEGIN
  SELECT RAISE(ABORT, 'transaction_deletion_events rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER transaction_deletion_events_no_delete
BEFORE DELETE ON transaction_deletion_events
BEGIN
  SELECT RAISE(ABORT, 'transaction_deletion_events rows are append-only');
END;
-- +goose StatementEnd

-- Same-day sequence columns retain their former DEFAULT 0, including for a
-- fresh database, to match the schema produced by the pre-beta chain.
-- +goose StatementBegin
DROP TRIGGER IF EXISTS transaction_versions_no_update;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TRIGGER IF EXISTS posting_versions_no_update;
-- +goose StatementEnd

ALTER TABLE transaction_versions
  ADD COLUMN transaction_day_sequence INTEGER NOT NULL DEFAULT 0;

ALTER TABLE posting_versions
  ADD COLUMN account_day_sequence INTEGER NOT NULL DEFAULT 0;

-- +goose StatementBegin
UPDATE transaction_versions
SET transaction_day_sequence = (
  SELECT COUNT(DISTINCT other.transaction_id)
  FROM transaction_versions other
  WHERE other.book_id = transaction_versions.book_id
    AND other.transaction_date = transaction_versions.transaction_date
    AND other.transaction_id <= transaction_versions.transaction_id
);
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE posting_versions
SET account_day_sequence = (
  SELECT COUNT(DISTINCT other.posting_line_id)
  FROM posting_versions other
  JOIN journal_entries other_je ON other_je.id = other.journal_entry_id
  JOIN journal_entries self_je  ON self_je.id  = posting_versions.journal_entry_id
  WHERE other.book_id = posting_versions.book_id
    AND other.account_id = posting_versions.account_id
    AND other_je.entry_date = self_je.entry_date
    AND other.posting_line_id <= posting_versions.posting_line_id
);
-- +goose StatementEnd

ALTER TABLE reconciliation_checkpoints
  ADD COLUMN statement_account_sequence INTEGER NOT NULL DEFAULT 0;

-- +goose StatementBegin
UPDATE reconciliation_checkpoints
SET statement_account_sequence = COALESCE((
  SELECT MAX(pv.account_day_sequence)
  FROM reconciliation_checkpoint_postings rcp
  JOIN posting_versions pv ON pv.id = rcp.posting_version_id
  WHERE rcp.checkpoint_id = reconciliation_checkpoints.id
    AND rcp.entry_date = reconciliation_checkpoints.statement_date
), 0);
-- +goose StatementEnd

CREATE INDEX transaction_versions_book_date_seq_idx
  ON transaction_versions (book_id, transaction_date DESC, transaction_day_sequence DESC, id DESC);

CREATE INDEX posting_versions_account_date_seq_idx
  ON posting_versions (book_id, account_id, account_day_sequence DESC, id DESC);

-- +goose StatementBegin
CREATE TRIGGER transaction_versions_no_update
BEFORE UPDATE ON transaction_versions
BEGIN
  SELECT RAISE(ABORT, 'transaction_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER posting_versions_no_update
BEFORE UPDATE ON posting_versions
BEGIN
  SELECT RAISE(ABORT, 'posting_versions rows are append-only');
END;
-- +goose StatementEnd

CREATE TABLE import_profiles (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  name TEXT NOT NULL CHECK (length(trim(name)) > 0 AND name = trim(name)),
  adapter_kind TEXT NOT NULL CHECK (length(trim(adapter_kind)) > 0),
  config_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(config_json)),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX import_profiles_book_idx ON import_profiles (book_id);

CREATE TABLE import_batches (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  source_kind TEXT NOT NULL CHECK (length(trim(source_kind)) > 0),
  profile_id INTEGER REFERENCES import_profiles(id) ON DELETE SET NULL,
  status TEXT NOT NULL DEFAULT 'previewing' CHECK (
    status IN ('previewing', 'committed', 'partially_committed', 'discarded', 'failed')
  ),
  original_filename TEXT NOT NULL DEFAULT '',
  source_meta_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(source_meta_json)),
  created_at TEXT NOT NULL
);

CREATE INDEX import_batches_book_created_idx ON import_batches (book_id, created_at DESC, id DESC);

CREATE TABLE import_batch_events (
  id INTEGER PRIMARY KEY,
  batch_id INTEGER NOT NULL REFERENCES import_batches(id) ON DELETE CASCADE,
  event_kind TEXT NOT NULL CHECK (
    event_kind IN ('created', 'parsed', 'committed', 'partially_committed', 'discarded', 'failed', 'rolled_back')
  ),
  detail_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(detail_json)),
  audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  created_at TEXT NOT NULL
);

CREATE INDEX import_batch_events_batch_idx ON import_batch_events (batch_id, created_at ASC, id ASC);

CREATE TABLE import_staged_rows (
  id INTEGER PRIMARY KEY,
  batch_id INTEGER NOT NULL REFERENCES import_batches(id) ON DELETE CASCADE,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  row_index INTEGER NOT NULL CHECK (row_index >= 0),
  dedupe_fingerprint TEXT NOT NULL,
  raw_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(raw_json)),
  normalized_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(normalized_json)),
  dedupe_status TEXT NOT NULL DEFAULT 'new' CHECK (
    dedupe_status IN ('new', 'duplicate', 'needs_attention', 'excluded')
  ),
  resolution_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(resolution_json)),
  commit_status TEXT NOT NULL DEFAULT 'pending' CHECK (
    commit_status IN ('pending', 'committed', 'skipped', 'failed')
  ),
  committed_transaction_id INTEGER REFERENCES transactions(id) ON DELETE SET NULL,
  commit_error TEXT,
  UNIQUE (batch_id, row_index)
);

CREATE INDEX import_staged_rows_batch_idx ON import_staged_rows (batch_id, row_index ASC);
CREATE INDEX import_staged_rows_book_fingerprint_idx ON import_staged_rows (book_id, dedupe_fingerprint);

CREATE TABLE import_commit_identities (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  dedupe_fingerprint TEXT NOT NULL,
  committed_transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE RESTRICT,
  source_kind TEXT NOT NULL,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  created_at TEXT NOT NULL,
  UNIQUE (book_id, dedupe_fingerprint)
);

CREATE INDEX import_commit_identities_book_idx ON import_commit_identities (book_id, created_at DESC);

ALTER TABLE account_versions ADD COLUMN cost_basis_method TEXT CHECK (
  cost_basis_method IS NULL OR cost_basis_method IN ('fifo', 'lifo', 'average_cost', 'specific_lot')
);

DROP VIEW IF EXISTS current_account_versions;
CREATE VIEW current_account_versions AS
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

CREATE UNIQUE INDEX background_work_items_active_unique_idx
  ON background_work_items (book_id, kind, payload_json)
  WHERE status IN ('pending', 'running');

DROP TRIGGER IF EXISTS fx_work_after_account_version_insert;
DROP TRIGGER IF EXISTS fx_work_after_posting_version_insert;

-- +goose StatementBegin
CREATE TRIGGER fx_work_after_account_version_insert
AFTER INSERT ON account_versions
WHEN NEW.status = 'active'
  AND EXISTS (SELECT 1 FROM commodities c WHERE c.id = NEW.default_commodity_id AND c.kind = 'currency')
  AND NEW.default_commodity_id != COALESCE(
    (SELECT pp.base_commodity_id FROM accounts a JOIN pricing_policies pp ON pp.book_id = a.book_id WHERE a.id = NEW.account_id),
    (SELECT b.default_currency_commodity_id FROM accounts a JOIN books b ON b.id = a.book_id WHERE a.id = NEW.account_id),
    0
  )
  AND NOT EXISTS (
    SELECT 1 FROM account_versions prior
    WHERE prior.account_id = NEW.account_id AND prior.id != NEW.id
      AND prior.status = 'active' AND prior.default_commodity_id = NEW.default_commodity_id
  )
  AND NOT EXISTS (
    SELECT 1 FROM current_account_versions other
    JOIN accounts other_account ON other_account.id = other.account_id
    JOIN accounts new_account ON new_account.id = NEW.account_id
    WHERE other_account.book_id = new_account.book_id
      AND other.account_id != NEW.account_id
      AND other.status = 'active' AND other.default_commodity_id = NEW.default_commodity_id
  )
BEGIN
  INSERT INTO background_work_items (book_id, kind, payload_json, available_at, created_at, updated_at)
  SELECT a.book_id, 'pricing.fx_coverage',
    json_object('reason', 'currency_activated', 'start_date', NEW.opened_on, 'currency_id', NEW.default_commodity_id),
    NEW.recorded_at, NEW.recorded_at, NEW.recorded_at
  FROM accounts a WHERE a.id = NEW.account_id
  ON CONFLICT(book_id, kind, payload_json) WHERE status IN ('pending', 'running') DO NOTHING;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER fx_work_after_posting_version_insert
AFTER INSERT ON posting_versions
WHEN EXISTS (SELECT 1 FROM transaction_versions tv WHERE tv.id = NEW.transaction_version_id AND tv.status = 'posted')
  AND EXISTS (SELECT 1 FROM commodities c WHERE c.id = NEW.commodity_id AND c.kind = 'currency')
  AND NEW.commodity_id != COALESCE(
    (SELECT base_commodity_id FROM pricing_policies WHERE book_id = NEW.book_id),
    (SELECT default_currency_commodity_id FROM books WHERE id = NEW.book_id),
    0
  )
  AND NOT EXISTS (
    SELECT 1 FROM background_work_items bw
    WHERE bw.book_id = NEW.book_id AND bw.kind = 'pricing.fx_coverage'
      AND bw.status IN ('pending', 'running')
      AND json_extract(bw.payload_json, '$.currency_id') = NEW.commodity_id
      AND json_extract(bw.payload_json, '$.start_date') <= (SELECT entry_date FROM journal_entries WHERE id = NEW.journal_entry_id)
  )
BEGIN
  INSERT INTO background_work_items (book_id, kind, payload_json, available_at, created_at, updated_at)
  VALUES (
    NEW.book_id, 'pricing.fx_coverage',
    json_object('reason', 'transaction_entered', 'start_date', (SELECT entry_date FROM journal_entries WHERE id = NEW.journal_entry_id), 'currency_id', NEW.commodity_id),
    strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
  )
  ON CONFLICT(book_id, kind, payload_json) WHERE status IN ('pending', 'running') DO NOTHING;
END;
-- +goose StatementEnd

CREATE TABLE import_connections (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  source_kind TEXT NOT NULL CHECK (length(trim(source_kind)) > 0),
  display_name TEXT NOT NULL CHECK (length(trim(display_name)) > 0 AND display_name = trim(display_name)),
  secret_ciphertext TEXT NOT NULL,
  config_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(config_json)),
  transactions_cursor TEXT NOT NULL DEFAULT '',
  orders_cursor TEXT NOT NULL DEFAULT '',
  dividends_cursor TEXT NOT NULL DEFAULT '',
  last_fetch_status TEXT NOT NULL DEFAULT '' CHECK (last_fetch_status IN ('', 'fetching', 'ready', 'failed')),
  last_fetched_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  auto_refresh_enabled INTEGER NOT NULL DEFAULT 0 CHECK (auto_refresh_enabled IN (0, 1)),
  cash_account_id INTEGER REFERENCES accounts(id) ON DELETE SET NULL,
  UNIQUE (book_id, source_kind, display_name)
);

CREATE INDEX import_connections_book_idx ON import_connections (book_id, created_at DESC, id DESC);

ALTER TABLE import_batches ADD COLUMN connection_id INTEGER REFERENCES import_connections(id) ON DELETE SET NULL;
CREATE INDEX import_batches_connection_idx ON import_batches (connection_id) WHERE connection_id IS NOT NULL;

CREATE TABLE import_connection_holdings (
  id INTEGER PRIMARY KEY,
  connection_id INTEGER NOT NULL REFERENCES import_connections(id) ON DELETE CASCADE,
  commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  holding_account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  created_at TEXT NOT NULL,
  UNIQUE (connection_id, commodity_id)
);

CREATE INDEX import_connection_holdings_connection_idx ON import_connection_holdings (connection_id);

DROP TRIGGER IF EXISTS investment_instrument_versions_no_delete;
DROP TRIGGER IF EXISTS commodity_versions_no_delete;

-- +goose StatementBegin
CREATE TRIGGER investment_instrument_versions_no_delete
BEFORE DELETE ON investment_instrument_versions
WHEN EXISTS (
  SELECT 1 FROM investment_instruments ii
  WHERE ii.id = OLD.instrument_id
    AND (
      EXISTS (SELECT 1 FROM investment_lots lot WHERE lot.commodity_id = ii.commodity_id)
      OR EXISTS (SELECT 1 FROM posting_versions pv WHERE pv.commodity_id = ii.commodity_id)
      OR EXISTS (SELECT 1 FROM import_connection_holdings ich WHERE ich.commodity_id = ii.commodity_id)
      OR EXISTS (SELECT 1 FROM dividend_defaults dd WHERE dd.commodity_id = ii.commodity_id)
      OR EXISTS (SELECT 1 FROM price_series ps WHERE ps.base_commodity_id = ii.commodity_id OR ps.quote_commodity_id = ii.commodity_id)
      OR EXISTS (SELECT 1 FROM instrument_source_links isl WHERE isl.instrument_id = ii.id)
      OR EXISTS (SELECT 1 FROM investment_provider_events ipe WHERE ipe.instrument_id = ii.id)
      OR EXISTS (SELECT 1 FROM investment_event_suggestions ies WHERE ies.instrument_id = ii.id)
      OR EXISTS (SELECT 1 FROM investment_automation_rules iar WHERE iar.instrument_id = ii.id)
    )
)
BEGIN
  SELECT RAISE(ABORT, 'investment_instrument_versions rows are append-only once referenced');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER commodity_versions_no_delete
BEFORE DELETE ON commodity_versions
WHEN EXISTS (
  SELECT 1 FROM commodities c
  WHERE c.id = OLD.commodity_id
    AND (
      c.kind != 'security'
      OR EXISTS (SELECT 1 FROM posting_versions pv WHERE pv.commodity_id = c.id)
      OR EXISTS (SELECT 1 FROM investment_lots lot WHERE lot.commodity_id = c.id OR lot.cost_commodity_id = c.id)
      OR EXISTS (SELECT 1 FROM import_connection_holdings ich WHERE ich.commodity_id = c.id)
      OR EXISTS (SELECT 1 FROM dividend_defaults dd WHERE dd.commodity_id = c.id)
      OR EXISTS (SELECT 1 FROM account_versions av WHERE av.default_commodity_id = c.id)
      OR EXISTS (SELECT 1 FROM price_series ps WHERE ps.base_commodity_id = c.id OR ps.quote_commodity_id = c.id)
      OR EXISTS (SELECT 1 FROM price_observations po WHERE po.base_commodity_id = c.id OR po.quote_commodity_id = c.id)
    )
)
BEGIN
  SELECT RAISE(ABORT, 'commodity_versions rows are append-only once referenced');
END;
-- +goose StatementEnd

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

-- Scheduled backups (R3 slice 4, docs/plans/data-portability-plan.md).
--
-- The policy is stored rather than configured by environment so the owner can
-- change it from the app; where the files go stays an operator decision
-- (BACKUP_DIR), because it is a deployment fact, not a preference.
CREATE TABLE IF NOT EXISTS backup_policies (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL UNIQUE REFERENCES books(id) ON DELETE RESTRICT,
  enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
  hour_local INTEGER NOT NULL DEFAULT 3 CHECK (hour_local BETWEEN 0 AND 23),
  minute_local INTEGER NOT NULL DEFAULT 15 CHECK (minute_local BETWEEN 0 AND 59),
  retention_count INTEGER NOT NULL DEFAULT 14 CHECK (retention_count >= 1),
  retention_max_age_days INTEGER CHECK (retention_max_age_days IS NULL OR retention_max_age_days >= 1),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

-- One row per backup occurrence, created in the same transaction as the work
-- item that will execute it.
--
-- occurrence_key is what makes a retry idempotent where the work queue cannot:
-- the queue's uniqueness covers pending and running items only, so once an item
-- completes the same scheduled day could be enqueued again. A scheduled run's
-- key is its local date; a manual run gets its own identity because asking for
-- a backup twice is a reasonable thing to do.
CREATE TABLE IF NOT EXISTS backup_runs (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  trigger TEXT NOT NULL CHECK (trigger IN ('scheduled', 'manual')),
  occurrence_key TEXT NOT NULL CHECK (length(trim(occurrence_key)) > 0),
  status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'failed')),
  -- Deterministic from the occurrence, not from the clock at execution time, so
  -- a retry writes to the same path instead of scattering near-duplicates.
  target_path TEXT NOT NULL,
  scheduled_for_local_date TEXT CHECK (scheduled_for_local_date IS NULL OR scheduled_for_local_date GLOB '????-??-??'),
  byte_size INTEGER,
  page_count INTEGER,
  verified INTEGER NOT NULL DEFAULT 0 CHECK (verified IN (0, 1)),
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  error_summary TEXT NOT NULL DEFAULT '',
  work_item_id INTEGER REFERENCES background_work_items(id) ON DELETE SET NULL,
  requested_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  started_at TEXT,
  finished_at TEXT,
  pruned_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (book_id, occurrence_key)
);

CREATE INDEX IF NOT EXISTS backup_runs_book_created_idx
  ON backup_runs (book_id, created_at DESC, id DESC);

-- Pruning walks completed, unpruned runs oldest-first.
CREATE INDEX IF NOT EXISTS backup_runs_retention_idx
  ON backup_runs (book_id, status, pruned_at, created_at, id);

-- The ledger self-check (R3 slice 6, docs/plans/data-portability-plan.md).
--
-- Read-only and diagnostic: it reports and explains, and never repairs. These
-- two tables are the only thing it writes, and they hold no ledger data — a
-- check that could change a balance would be a different, far more dangerous
-- feature.
CREATE TABLE IF NOT EXISTS self_check_runs (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  trigger TEXT NOT NULL CHECK (trigger IN ('manual', 'scheduled')),
  status TEXT NOT NULL CHECK (status IN ('running', 'passed', 'failed')),
  failed_check_count INTEGER NOT NULL DEFAULT 0 CHECK (failed_check_count >= 0),
  started_at TEXT NOT NULL,
  finished_at TEXT,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS self_check_runs_book_started_idx
  ON self_check_runs (book_id, started_at DESC, id DESC);

-- One row per check per run. A table rather than a JSON blob on the run,
-- because "which check failed, how often, since when" is a question worth
-- answering with a query — the mistake T-54 records about price derivations.
CREATE TABLE IF NOT EXISTS self_check_results (
  id INTEGER PRIMARY KEY,
  run_id INTEGER NOT NULL REFERENCES self_check_runs(id) ON DELETE CASCADE,
  check_id TEXT NOT NULL CHECK (length(trim(check_id)) > 0),
  status TEXT NOT NULL CHECK (status IN ('passed', 'failed', 'not_applicable')),
  finding_count INTEGER NOT NULL DEFAULT 0 CHECK (finding_count >= 0),
  -- A capped sample of offending identifiers, for a human to go and look at.
  -- Diagnostic display only: never joined on, never counted from.
  sample_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(sample_json)),
  summary TEXT NOT NULL DEFAULT '',
  UNIQUE (run_id, check_id)
);

CREATE INDEX IF NOT EXISTS self_check_results_check_idx
  ON self_check_results (check_id, status);

-- +goose Down
DROP INDEX IF EXISTS self_check_results_check_idx;
DROP TABLE IF EXISTS self_check_results;
DROP INDEX IF EXISTS self_check_runs_book_started_idx;
DROP TABLE IF EXISTS self_check_runs;
DROP INDEX IF EXISTS backup_runs_retention_idx;
DROP INDEX IF EXISTS backup_runs_book_created_idx;
DROP TABLE IF EXISTS backup_runs;
DROP TABLE IF EXISTS backup_policies;
DROP INDEX IF EXISTS login_mfa_challenges_expiry_idx;
DROP TABLE IF EXISTS login_mfa_challenges;
DROP INDEX IF EXISTS user_mfa_recovery_codes_user_idx;
DROP TABLE IF EXISTS user_mfa_recovery_codes;
DROP TABLE IF EXISTS user_mfa_totp;
DROP INDEX IF EXISTS login_trusted_devices_user_idx;
DROP TABLE IF EXISTS login_trusted_devices;
DROP INDEX IF EXISTS authentication_events_client_ip_idx;
DROP INDEX IF EXISTS authentication_events_occurred_idx;
DROP TABLE IF EXISTS authentication_events;
DROP TRIGGER IF EXISTS commodity_versions_no_delete;
DROP TRIGGER IF EXISTS investment_instrument_versions_no_delete;
DROP INDEX IF EXISTS import_connection_holdings_connection_idx;
DROP TABLE IF EXISTS import_connection_holdings;
DROP INDEX IF EXISTS import_batches_connection_idx;
DROP INDEX IF EXISTS import_connections_book_idx;
DROP TABLE IF EXISTS import_connections;
DROP INDEX IF EXISTS background_work_items_active_unique_idx;
DROP INDEX IF EXISTS import_commit_identities_book_idx;
DROP TABLE IF EXISTS import_commit_identities;
DROP INDEX IF EXISTS import_staged_rows_book_fingerprint_idx;
DROP INDEX IF EXISTS import_staged_rows_batch_idx;
DROP TABLE IF EXISTS import_staged_rows;
DROP INDEX IF EXISTS import_batch_events_batch_idx;
DROP TABLE IF EXISTS import_batch_events;
DROP INDEX IF EXISTS import_batches_book_created_idx;
DROP TABLE IF EXISTS import_batches;
DROP INDEX IF EXISTS import_profiles_book_idx;
DROP TABLE IF EXISTS import_profiles;
DROP INDEX IF EXISTS posting_versions_account_date_seq_idx;
DROP INDEX IF EXISTS transaction_versions_book_date_seq_idx;
DROP TRIGGER IF EXISTS transaction_deletion_events_no_delete;
DROP TRIGGER IF EXISTS transaction_deletion_events_no_update;
DROP INDEX IF EXISTS transaction_deletion_events_transaction_idx;
DROP TABLE IF EXISTS transaction_deletion_events;
DROP INDEX IF EXISTS transactions_deleted_idx;
DROP TRIGGER IF EXISTS fx_work_after_posting_version_insert;
DROP TRIGGER IF EXISTS fx_work_after_account_version_insert;
DROP TRIGGER IF EXISTS price_observations_positive_update;
DROP TRIGGER IF EXISTS price_observations_positive_insert;
DROP TRIGGER IF EXISTS price_observations_same_book;
DROP TRIGGER IF EXISTS price_series_same_book;
DROP TRIGGER IF EXISTS pricing_source_assignments_same_book;
DROP TRIGGER IF EXISTS investment_lots_values_valid_update;
DROP TRIGGER IF EXISTS investment_lots_values_valid_insert;
DROP TRIGGER IF EXISTS investment_lot_events_same_book;
DROP TRIGGER IF EXISTS investment_lots_same_book;
DROP TRIGGER IF EXISTS investment_instruments_commodity_must_be_security;
DROP TRIGGER IF EXISTS investment_instrument_versions_no_delete;
DROP TRIGGER IF EXISTS investment_instrument_versions_no_update;
DROP TRIGGER IF EXISTS posting_tags_same_book_and_active_tag;
DROP TRIGGER IF EXISTS transaction_tags_same_book_and_active_tag;
DROP TRIGGER IF EXISTS posting_versions_commodity_scale_valid;
DROP TRIGGER IF EXISTS posting_versions_account_version_valid;
DROP TRIGGER IF EXISTS posting_versions_same_book_and_lineage;
DROP TRIGGER IF EXISTS posting_lines_same_book;
DROP TRIGGER IF EXISTS journal_entries_same_book;
DROP TRIGGER IF EXISTS transactions_correction_same_book;
DROP TRIGGER IF EXISTS transaction_search_insert;
DROP TRIGGER IF EXISTS transaction_versions_supersedes_same_transaction;
DROP TRIGGER IF EXISTS transaction_versions_same_book;
DROP TRIGGER IF EXISTS posting_versions_no_delete;
DROP TRIGGER IF EXISTS posting_versions_no_update;
DROP TRIGGER IF EXISTS journal_entries_no_delete;
DROP TRIGGER IF EXISTS journal_entries_no_update;
DROP TRIGGER IF EXISTS transaction_versions_no_delete;
DROP TRIGGER IF EXISTS transaction_versions_no_update;
DROP TRIGGER IF EXISTS payee_versions_no_delete;
DROP TRIGGER IF EXISTS payee_versions_no_update;
DROP TRIGGER IF EXISTS account_versions_no_delete;
DROP TRIGGER IF EXISTS account_versions_no_update;
DROP TRIGGER IF EXISTS accounts_system_fields_no_update;
DROP TRIGGER IF EXISTS institution_versions_no_delete;
DROP TRIGGER IF EXISTS institution_versions_no_update;
DROP TRIGGER IF EXISTS commodity_versions_no_delete;
DROP TRIGGER IF EXISTS commodity_versions_no_update;
DROP TRIGGER IF EXISTS commodity_versions_kind_scale_valid;
DROP TRIGGER IF EXISTS books_default_currency_must_exist;
DROP TRIGGER IF EXISTS books_default_currency_must_exist_on_insert;

DROP INDEX IF EXISTS background_work_items_due_idx;
DROP TABLE IF EXISTS background_work_items;

DROP INDEX IF EXISTS investment_automation_rules_lookup_idx;
DROP TABLE IF EXISTS investment_automation_rules;
DROP INDEX IF EXISTS investment_event_suggestions_book_idx;
DROP TABLE IF EXISTS investment_event_suggestions;
DROP INDEX IF EXISTS investment_provider_events_book_idx;
DROP INDEX IF EXISTS investment_provider_events_source_event_idx;
DROP TABLE IF EXISTS investment_provider_events;
DROP INDEX IF EXISTS investment_lot_events_lot_idx;
DROP TABLE IF EXISTS investment_lot_events;
DROP INDEX IF EXISTS investment_lots_position_idx;
DROP TABLE IF EXISTS investment_lots;
DROP INDEX IF EXISTS dividend_defaults_lookup_idx;
DROP TABLE IF EXISTS dividend_defaults;
DROP INDEX IF EXISTS cost_basis_profiles_default_idx;
DROP TABLE IF EXISTS cost_basis_profiles;
DROP TABLE IF EXISTS pricing_refresh_state;
DROP INDEX IF EXISTS market_data_ingest_items_run_idx;
DROP TABLE IF EXISTS market_data_ingest_items;
DROP INDEX IF EXISTS market_data_ingest_runs_book_idx;
DROP TABLE IF EXISTS market_data_ingest_runs;
DROP INDEX IF EXISTS price_observations_provider_event_idx;
DROP INDEX IF EXISTS price_observations_lookup_idx;
DROP TABLE IF EXISTS price_observations;
DROP INDEX IF EXISTS price_series_provider_idx;
DROP INDEX IF EXISTS price_series_lookup_idx;
DROP TABLE IF EXISTS price_series;
DROP INDEX IF EXISTS pricing_source_assignments_lookup_idx;
DROP TABLE IF EXISTS pricing_source_assignments;
DROP TABLE IF EXISTS pricing_policies;
DROP INDEX IF EXISTS instrument_search_cache_query_idx;
DROP TABLE IF EXISTS instrument_search_cache;
DROP INDEX IF EXISTS instrument_source_links_active_provider_idx;
DROP INDEX IF EXISTS instrument_source_links_lookup_idx;
DROP TABLE IF EXISTS instrument_source_links;
DROP INDEX IF EXISTS investment_instrument_versions_symbol_idx;
DROP INDEX IF EXISTS investment_instrument_versions_instrument_seq_idx;
DROP VIEW IF EXISTS current_investment_instrument_versions;
DROP TABLE IF EXISTS investment_instrument_versions;
DROP INDEX IF EXISTS investment_instruments_book_idx;
DROP TABLE IF EXISTS investment_instruments;
DROP TABLE IF EXISTS market_data_sources;

DROP TABLE IF EXISTS user_preferences;

DROP INDEX IF EXISTS reconciliation_checkpoint_postings_posting_idx;
DROP TABLE IF EXISTS reconciliation_checkpoint_postings;
DROP INDEX IF EXISTS reconciliation_checkpoints_account_idx;
DROP TABLE IF EXISTS reconciliation_checkpoints;
DROP INDEX IF EXISTS reconciliation_session_postings_posting_idx;
DROP TABLE IF EXISTS reconciliation_session_postings;
DROP INDEX IF EXISTS reconciliation_sessions_account_idx;
DROP TABLE IF EXISTS reconciliation_sessions;
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
