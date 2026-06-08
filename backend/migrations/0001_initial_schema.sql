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

CREATE TABLE IF NOT EXISTS audit_event_targets (
  audit_event_id INTEGER NOT NULL REFERENCES audit_events(id) ON DELETE CASCADE,
  book_id INTEGER REFERENCES books(id) ON DELETE RESTRICT,
  target_type TEXT NOT NULL CHECK (length(trim(target_type)) > 0 AND target_type = trim(target_type)),
  target_id INTEGER NOT NULL,
  target_role TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (audit_event_id, target_type, target_id, target_role)
);

CREATE INDEX IF NOT EXISTS audit_event_targets_target_idx
  ON audit_event_targets (book_id, target_type, target_id, audit_event_id DESC);

CREATE TABLE IF NOT EXISTS audit_event_changes (
  id INTEGER PRIMARY KEY,
  audit_event_id INTEGER NOT NULL REFERENCES audit_events(id) ON DELETE CASCADE,
  book_id INTEGER REFERENCES books(id) ON DELETE RESTRICT,
  table_name TEXT NOT NULL CHECK (length(trim(table_name)) > 0 AND table_name = trim(table_name)),
  row_id INTEGER NOT NULL,
  operation TEXT NOT NULL CHECK (operation IN ('insert', 'update', 'delete')),
  before_json TEXT CHECK (before_json IS NULL OR json_valid(before_json)),
  after_json TEXT CHECK (after_json IS NULL OR json_valid(after_json)),
  changed_fields_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(changed_fields_json))
);

CREATE INDEX IF NOT EXISTS audit_event_changes_row_idx
  ON audit_event_changes (book_id, table_name, row_id, audit_event_id DESC);

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
  default_currency_commodity_id INTEGER REFERENCES commodities(id) ON DELETE RESTRICT,
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
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
  symbol TEXT NOT NULL DEFAULT '',
  display_symbol TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL DEFAULT '',
  standard_scale INTEGER NOT NULL DEFAULT 2 CHECK (standard_scale BETWEEN 0 AND 12),
  max_quantity_scale INTEGER NOT NULL DEFAULT 2 CHECK (max_quantity_scale BETWEEN 0 AND 12 AND max_quantity_scale >= standard_scale),
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL DEFAULT '',
  updated_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  change_reason TEXT NOT NULL DEFAULT '',
  created_request_id TEXT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  UNIQUE (book_id, code)
);

CREATE INDEX IF NOT EXISTS commodities_book_kind_idx
  ON commodities (book_id, kind, code);

CREATE TABLE IF NOT EXISTS institutions (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
  name TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL DEFAULT 'other' CHECK (kind IN ('bank', 'credit_union', 'brokerage', 'card_issuer', 'lender', 'insurance', 'employer', 'rewards_program', 'government', 'other')),
  country_code TEXT,
  website TEXT,
  address_json TEXT NOT NULL DEFAULT '{}',
  comment_markdown TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL DEFAULT '',
  updated_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  change_reason TEXT NOT NULL DEFAULT '',
  created_request_id TEXT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS institutions_book_idx
  ON institutions (book_id, status, name COLLATE NOCASE);

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
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'closed', 'archived')),
  opened_on TEXT NOT NULL DEFAULT '0001-01-01' CHECK (opened_on GLOB '????-??-??'),
  closed_on TEXT CHECK (closed_on IS NULL OR closed_on GLOB '????-??-??'),
  code TEXT,
  name TEXT,
  account_class TEXT NOT NULL DEFAULT 'asset' CHECK (account_class IN ('asset', 'liability', 'equity', 'income', 'expense')),
  account_kind TEXT NOT NULL DEFAULT 'cash' REFERENCES account_kinds(code) ON DELETE RESTRICT,
  parent_account_id INTEGER REFERENCES accounts(id) ON DELETE RESTRICT,
  institution_id INTEGER REFERENCES institutions(id) ON DELETE RESTRICT,
  country_code TEXT,
  default_commodity_id INTEGER REFERENCES commodities(id) ON DELETE RESTRICT,
  quantity_scale_override INTEGER CHECK (quantity_scale_override IS NULL OR quantity_scale_override BETWEEN 0 AND 12),
  allows_postings INTEGER NOT NULL DEFAULT 0 CHECK (allows_postings IN (0, 1)),
  number_last4 TEXT,
  external_ref_hint TEXT,
  comment_markdown TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL DEFAULT '',
  updated_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  change_reason TEXT NOT NULL DEFAULT '',
  created_request_id TEXT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  CHECK (closed_on IS NULL OR closed_on >= opened_on)
);

CREATE UNIQUE INDEX IF NOT EXISTS accounts_system_role_idx
  ON accounts (book_id, system_role)
  WHERE system_role IS NOT NULL;

CREATE INDEX IF NOT EXISTS accounts_book_status_class_idx
  ON accounts (book_id, status, account_class, name COLLATE NOCASE);

CREATE INDEX IF NOT EXISTS accounts_parent_idx
  ON accounts (parent_account_id)
  WHERE parent_account_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS accounts_institution_idx
  ON accounts (institution_id)
  WHERE institution_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS payees (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
  name TEXT NOT NULL DEFAULT '',
  normalized_name TEXT NOT NULL DEFAULT '',
  default_account_id INTEGER REFERENCES accounts(id) ON DELETE RESTRICT,
  default_category_account_id INTEGER REFERENCES accounts(id) ON DELETE RESTRICT,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL DEFAULT '',
  updated_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  change_reason TEXT NOT NULL DEFAULT '',
  created_request_id TEXT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS payees_book_status_name_idx
  ON payees (book_id, status, name COLLATE NOCASE);

CREATE UNIQUE INDEX IF NOT EXISTS payees_active_normalized_name_idx
  ON payees (book_id, normalized_name)
  WHERE status = 'active';

CREATE TABLE IF NOT EXISTS transactions (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  correction_of_transaction_id INTEGER REFERENCES transactions(id) ON DELETE RESTRICT,
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'posted', 'voided')),
  transaction_kind TEXT NOT NULL DEFAULT 'ordinary' CHECK (transaction_kind IN ('ordinary', 'transfer', 'investment', 'opening_balance', 'adjustment')),
  transaction_date TEXT NOT NULL DEFAULT '0001-01-01' CHECK (transaction_date GLOB '????-??-??'),
  payee_id INTEGER REFERENCES payees(id) ON DELETE RESTRICT,
  payee_name TEXT,
  description TEXT NOT NULL DEFAULT '',
  external_ref_hint TEXT,
  note_markdown TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL DEFAULT '',
  updated_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  created_request_id TEXT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  change_reason TEXT NOT NULL DEFAULT '',
  CHECK (correction_of_transaction_id IS NULL OR correction_of_transaction_id <> id)
);

CREATE INDEX IF NOT EXISTS transactions_book_date_idx
  ON transactions (book_id, transaction_date DESC, id DESC);

CREATE INDEX IF NOT EXISTS transactions_correction_idx
  ON transactions (correction_of_transaction_id)
  WHERE correction_of_transaction_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS transactions_payee_idx
  ON transactions (payee_id)
  WHERE payee_id IS NOT NULL;

CREATE VIRTUAL TABLE IF NOT EXISTS transaction_search
USING fts5(
  transaction_id UNINDEXED,
  payee_name,
  description,
  external_ref_hint,
  note_markdown
);

CREATE TABLE IF NOT EXISTS journal_entries (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
  entry_seq INTEGER NOT NULL CHECK (entry_seq > 0),
  entry_date TEXT NOT NULL CHECK (entry_date GLOB '????-??-??'),
  entry_kind TEXT NOT NULL CHECK (entry_kind IN ('ordinary', 'transfer_leg', 'exchange', 'investment', 'opening_balance', 'adjustment')),
  memo TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  UNIQUE (transaction_id, entry_seq)
);

CREATE INDEX IF NOT EXISTS journal_entries_transaction_idx
  ON journal_entries (transaction_id, entry_seq);

CREATE INDEX IF NOT EXISTS journal_entries_book_date_idx
  ON journal_entries (book_id, entry_date DESC, id DESC);

CREATE TABLE IF NOT EXISTS posting_lines (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
  line_key TEXT NOT NULL CHECK (length(trim(line_key)) > 0 AND line_key = trim(line_key)),
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_request_id TEXT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  UNIQUE (book_id, transaction_id, line_key)
);

CREATE INDEX IF NOT EXISTS posting_lines_transaction_idx
  ON posting_lines (transaction_id);

CREATE TABLE IF NOT EXISTS postings (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
  journal_entry_id INTEGER NOT NULL REFERENCES journal_entries(id) ON DELETE CASCADE,
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
  UNIQUE (journal_entry_id, line_seq),
  UNIQUE (transaction_id, posting_line_id)
);

CREATE INDEX IF NOT EXISTS postings_transaction_idx
  ON postings (transaction_id);

CREATE INDEX IF NOT EXISTS postings_account_idx
  ON postings (account_id, id);

CREATE INDEX IF NOT EXISTS postings_journal_idx
  ON postings (journal_entry_id, line_seq);

CREATE TABLE IF NOT EXISTS transaction_tags (
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
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
  posting_line_id INTEGER NOT NULL REFERENCES posting_lines(id) ON DELETE CASCADE,
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
  statement_balance_value INTEGER NOT NULL,
  statement_balance_scale INTEGER NOT NULL CHECK (statement_balance_scale BETWEEN 0 AND 12),
  starting_checkpoint_id INTEGER REFERENCES reconciliation_checkpoints(id) ON DELETE RESTRICT,
  starting_balance_value INTEGER NOT NULL,
  starting_balance_scale INTEGER NOT NULL CHECK (starting_balance_scale BETWEEN 0 AND 12),
  selected_balance_value INTEGER NOT NULL DEFAULT 0,
  selected_balance_scale INTEGER NOT NULL DEFAULT 0 CHECK (selected_balance_scale BETWEEN 0 AND 12),
  difference_value INTEGER NOT NULL DEFAULT 0,
  difference_scale INTEGER NOT NULL DEFAULT 0 CHECK (difference_scale BETWEEN 0 AND 12),
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
  session_id INTEGER NOT NULL REFERENCES reconciliation_sessions(id) ON DELETE CASCADE,
  posting_id INTEGER NOT NULL REFERENCES postings(id) ON DELETE RESTRICT,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE RESTRICT,
  posting_line_id INTEGER NOT NULL REFERENCES posting_lines(id) ON DELETE RESTRICT,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  entry_date TEXT NOT NULL CHECK (entry_date GLOB '????-??-??'),
  quantity_value INTEGER NOT NULL,
  quantity_scale INTEGER NOT NULL CHECK (quantity_scale BETWEEN 0 AND 12),
  payee_name TEXT,
  description TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  PRIMARY KEY (session_id, posting_id)
);

CREATE INDEX IF NOT EXISTS reconciliation_session_postings_posting_idx
  ON reconciliation_session_postings (posting_id);

CREATE TABLE IF NOT EXISTS reconciliation_checkpoints (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  session_id INTEGER REFERENCES reconciliation_sessions(id) ON DELETE RESTRICT,
  previous_checkpoint_id INTEGER REFERENCES reconciliation_checkpoints(id) ON DELETE RESTRICT,
  status TEXT NOT NULL CHECK (status IN ('active', 'invalidated', 'voided')),
  statement_date TEXT NOT NULL CHECK (statement_date GLOB '????-??-??'),
  statement_balance_value INTEGER NOT NULL,
  statement_balance_scale INTEGER NOT NULL CHECK (statement_balance_scale BETWEEN 0 AND 12),
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
  checkpoint_id INTEGER NOT NULL REFERENCES reconciliation_checkpoints(id) ON DELETE CASCADE,
  posting_id INTEGER NOT NULL REFERENCES postings(id) ON DELETE RESTRICT,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  entry_date TEXT NOT NULL CHECK (entry_date GLOB '????-??-??'),
  quantity_value INTEGER NOT NULL,
  quantity_scale INTEGER NOT NULL CHECK (quantity_scale BETWEEN 0 AND 12),
  PRIMARY KEY (checkpoint_id, posting_id)
);

CREATE INDEX IF NOT EXISTS reconciliation_checkpoint_postings_posting_idx
  ON reconciliation_checkpoint_postings (posting_id);

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS transaction_search_insert
AFTER INSERT ON transactions
BEGIN
  INSERT INTO transaction_search (
    transaction_id,
    payee_name,
    description,
    external_ref_hint,
    note_markdown
  )
  VALUES (
    NEW.id,
    COALESCE(NEW.payee_name, ''),
    COALESCE(NEW.description, ''),
    COALESCE(NEW.external_ref_hint, ''),
    COALESCE(NEW.note_markdown, '')
  );
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS transaction_search_update
AFTER UPDATE OF payee_name, description, external_ref_hint, note_markdown ON transactions
BEGIN
  DELETE FROM transaction_search WHERE transaction_id = OLD.id;
  INSERT INTO transaction_search (
    transaction_id,
    payee_name,
    description,
    external_ref_hint,
    note_markdown
  )
  VALUES (
    NEW.id,
    COALESCE(NEW.payee_name, ''),
    COALESCE(NEW.description, ''),
    COALESCE(NEW.external_ref_hint, ''),
    COALESCE(NEW.note_markdown, '')
  );
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS transaction_search_delete
AFTER DELETE ON transactions
BEGIN
  DELETE FROM transaction_search WHERE transaction_id = OLD.id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS audit_accounts_insert
AFTER INSERT ON accounts
WHEN NEW.created_audit_event_id IS NOT NULL
BEGIN
  INSERT OR IGNORE INTO audit_event_targets (audit_event_id, book_id, target_type, target_id, target_role)
  VALUES (NEW.created_audit_event_id, NEW.book_id, 'account', NEW.id, 'primary');
  INSERT INTO audit_event_changes (audit_event_id, book_id, table_name, row_id, operation, before_json, after_json, changed_fields_json)
  VALUES (
    NEW.created_audit_event_id,
    NEW.book_id,
    'accounts',
    NEW.id,
    'insert',
    NULL,
    json_object('id', NEW.id, 'book_id', NEW.book_id, 'status', NEW.status, 'opened_on', NEW.opened_on, 'closed_on', NEW.closed_on, 'code', NEW.code, 'name', NEW.name, 'account_class', NEW.account_class, 'account_kind', NEW.account_kind, 'parent_account_id', NEW.parent_account_id, 'institution_id', NEW.institution_id, 'default_commodity_id', NEW.default_commodity_id, 'allows_postings', NEW.allows_postings),
    json_array('id', 'book_id', 'status', 'opened_on', 'closed_on', 'code', 'name', 'account_class', 'account_kind', 'parent_account_id', 'institution_id', 'default_commodity_id', 'allows_postings')
  );
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS audit_accounts_update
AFTER UPDATE ON accounts
WHEN NEW.updated_audit_event_id IS NOT NULL
BEGIN
  INSERT OR IGNORE INTO audit_event_targets (audit_event_id, book_id, target_type, target_id, target_role)
  VALUES (NEW.updated_audit_event_id, NEW.book_id, 'account', NEW.id, 'primary');
  INSERT INTO audit_event_changes (audit_event_id, book_id, table_name, row_id, operation, before_json, after_json, changed_fields_json)
  VALUES (
    NEW.updated_audit_event_id,
    NEW.book_id,
    'accounts',
    NEW.id,
    'update',
    json_object('id', OLD.id, 'book_id', OLD.book_id, 'status', OLD.status, 'opened_on', OLD.opened_on, 'closed_on', OLD.closed_on, 'code', OLD.code, 'name', OLD.name, 'account_class', OLD.account_class, 'account_kind', OLD.account_kind, 'parent_account_id', OLD.parent_account_id, 'institution_id', OLD.institution_id, 'default_commodity_id', OLD.default_commodity_id, 'allows_postings', OLD.allows_postings),
    json_object('id', NEW.id, 'book_id', NEW.book_id, 'status', NEW.status, 'opened_on', NEW.opened_on, 'closed_on', NEW.closed_on, 'code', NEW.code, 'name', NEW.name, 'account_class', NEW.account_class, 'account_kind', NEW.account_kind, 'parent_account_id', NEW.parent_account_id, 'institution_id', NEW.institution_id, 'default_commodity_id', NEW.default_commodity_id, 'allows_postings', NEW.allows_postings),
    json_array('status', 'opened_on', 'closed_on', 'code', 'name', 'account_class', 'account_kind', 'parent_account_id', 'institution_id', 'default_commodity_id', 'allows_postings')
  );
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS audit_institutions_insert
AFTER INSERT ON institutions
WHEN NEW.created_audit_event_id IS NOT NULL
BEGIN
  INSERT OR IGNORE INTO audit_event_targets (audit_event_id, book_id, target_type, target_id, target_role)
  VALUES (NEW.created_audit_event_id, NEW.book_id, 'institution', NEW.id, 'primary');
  INSERT INTO audit_event_changes (audit_event_id, book_id, table_name, row_id, operation, before_json, after_json, changed_fields_json)
  VALUES (
    NEW.created_audit_event_id,
    NEW.book_id,
    'institutions',
    NEW.id,
    'insert',
    NULL,
    json_object('id', NEW.id, 'book_id', NEW.book_id, 'status', NEW.status, 'name', NEW.name, 'kind', NEW.kind, 'country_code', NEW.country_code, 'website', NEW.website),
    json_array('id', 'book_id', 'status', 'name', 'kind', 'country_code', 'website')
  );
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS audit_institutions_update
AFTER UPDATE ON institutions
WHEN NEW.updated_audit_event_id IS NOT NULL
BEGIN
  INSERT OR IGNORE INTO audit_event_targets (audit_event_id, book_id, target_type, target_id, target_role)
  VALUES (NEW.updated_audit_event_id, NEW.book_id, 'institution', NEW.id, 'primary');
  INSERT INTO audit_event_changes (audit_event_id, book_id, table_name, row_id, operation, before_json, after_json, changed_fields_json)
  VALUES (
    NEW.updated_audit_event_id,
    NEW.book_id,
    'institutions',
    NEW.id,
    'update',
    json_object('id', OLD.id, 'book_id', OLD.book_id, 'status', OLD.status, 'name', OLD.name, 'kind', OLD.kind, 'country_code', OLD.country_code, 'website', OLD.website),
    json_object('id', NEW.id, 'book_id', NEW.book_id, 'status', NEW.status, 'name', NEW.name, 'kind', NEW.kind, 'country_code', NEW.country_code, 'website', NEW.website),
    json_array('status', 'name', 'kind', 'country_code', 'website', 'address_json', 'comment_markdown', 'metadata_json')
  );
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS audit_commodities_insert
AFTER INSERT ON commodities
WHEN NEW.created_audit_event_id IS NOT NULL
BEGIN
  INSERT OR IGNORE INTO audit_event_targets (audit_event_id, book_id, target_type, target_id, target_role)
  VALUES (NEW.created_audit_event_id, NEW.book_id, 'commodity', NEW.id, 'primary');
  INSERT INTO audit_event_changes (audit_event_id, book_id, table_name, row_id, operation, before_json, after_json, changed_fields_json)
  VALUES (
    NEW.created_audit_event_id,
    NEW.book_id,
    'commodities',
    NEW.id,
    'insert',
    NULL,
    json_object('id', NEW.id, 'book_id', NEW.book_id, 'code', NEW.code, 'kind', NEW.kind, 'status', NEW.status, 'symbol', NEW.symbol, 'display_symbol', NEW.display_symbol, 'name', NEW.name, 'standard_scale', NEW.standard_scale, 'max_quantity_scale', NEW.max_quantity_scale),
    json_array('id', 'book_id', 'code', 'kind', 'status', 'symbol', 'display_symbol', 'name', 'standard_scale', 'max_quantity_scale')
  );
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS audit_commodities_update
AFTER UPDATE ON commodities
WHEN NEW.updated_audit_event_id IS NOT NULL
BEGIN
  INSERT OR IGNORE INTO audit_event_targets (audit_event_id, book_id, target_type, target_id, target_role)
  VALUES (NEW.updated_audit_event_id, NEW.book_id, 'commodity', NEW.id, 'primary');
  INSERT INTO audit_event_changes (audit_event_id, book_id, table_name, row_id, operation, before_json, after_json, changed_fields_json)
  VALUES (
    NEW.updated_audit_event_id,
    NEW.book_id,
    'commodities',
    NEW.id,
    'update',
    json_object('id', OLD.id, 'book_id', OLD.book_id, 'code', OLD.code, 'kind', OLD.kind, 'status', OLD.status, 'symbol', OLD.symbol, 'display_symbol', OLD.display_symbol, 'name', OLD.name, 'standard_scale', OLD.standard_scale, 'max_quantity_scale', OLD.max_quantity_scale),
    json_object('id', NEW.id, 'book_id', NEW.book_id, 'code', NEW.code, 'kind', NEW.kind, 'status', NEW.status, 'symbol', NEW.symbol, 'display_symbol', NEW.display_symbol, 'name', NEW.name, 'standard_scale', NEW.standard_scale, 'max_quantity_scale', NEW.max_quantity_scale),
    json_array('status', 'symbol', 'display_symbol', 'name', 'standard_scale', 'max_quantity_scale', 'metadata_json')
  );
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS audit_payees_insert
AFTER INSERT ON payees
WHEN NEW.created_audit_event_id IS NOT NULL
BEGIN
  INSERT OR IGNORE INTO audit_event_targets (audit_event_id, book_id, target_type, target_id, target_role)
  VALUES (NEW.created_audit_event_id, NEW.book_id, 'payee', NEW.id, 'primary');
  INSERT INTO audit_event_changes (audit_event_id, book_id, table_name, row_id, operation, before_json, after_json, changed_fields_json)
  VALUES (
    NEW.created_audit_event_id,
    NEW.book_id,
    'payees',
    NEW.id,
    'insert',
    NULL,
    json_object('id', NEW.id, 'book_id', NEW.book_id, 'status', NEW.status, 'name', NEW.name, 'default_account_id', NEW.default_account_id, 'default_category_account_id', NEW.default_category_account_id),
    json_array('id', 'book_id', 'status', 'name', 'default_account_id', 'default_category_account_id')
  );
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS audit_payees_update
AFTER UPDATE ON payees
WHEN NEW.updated_audit_event_id IS NOT NULL
BEGIN
  INSERT OR IGNORE INTO audit_event_targets (audit_event_id, book_id, target_type, target_id, target_role)
  VALUES (NEW.updated_audit_event_id, NEW.book_id, 'payee', NEW.id, 'primary');
  INSERT INTO audit_event_changes (audit_event_id, book_id, table_name, row_id, operation, before_json, after_json, changed_fields_json)
  VALUES (
    NEW.updated_audit_event_id,
    NEW.book_id,
    'payees',
    NEW.id,
    'update',
    json_object('id', OLD.id, 'book_id', OLD.book_id, 'status', OLD.status, 'name', OLD.name, 'default_account_id', OLD.default_account_id, 'default_category_account_id', OLD.default_category_account_id),
    json_object('id', NEW.id, 'book_id', NEW.book_id, 'status', NEW.status, 'name', NEW.name, 'default_account_id', NEW.default_account_id, 'default_category_account_id', NEW.default_category_account_id),
    json_array('status', 'name', 'default_account_id', 'default_category_account_id', 'metadata_json')
  );
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS audit_transactions_insert
AFTER INSERT ON transactions
WHEN NEW.created_audit_event_id IS NOT NULL
BEGIN
  INSERT OR IGNORE INTO audit_event_targets (audit_event_id, book_id, target_type, target_id, target_role)
  VALUES (NEW.created_audit_event_id, NEW.book_id, 'transaction', NEW.id, 'primary');
  INSERT INTO audit_event_changes (audit_event_id, book_id, table_name, row_id, operation, before_json, after_json, changed_fields_json)
  VALUES (
    NEW.created_audit_event_id,
    NEW.book_id,
    'transactions',
    NEW.id,
    'insert',
    NULL,
    json_object('id', NEW.id, 'book_id', NEW.book_id, 'status', NEW.status, 'transaction_kind', NEW.transaction_kind, 'transaction_date', NEW.transaction_date, 'payee_id', NEW.payee_id, 'payee_name', NEW.payee_name, 'description', NEW.description),
    json_array('id', 'book_id', 'status', 'transaction_kind', 'transaction_date', 'payee_id', 'payee_name', 'description')
  );
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS audit_transactions_update
AFTER UPDATE ON transactions
WHEN NEW.updated_audit_event_id IS NOT NULL
BEGIN
  INSERT OR IGNORE INTO audit_event_targets (audit_event_id, book_id, target_type, target_id, target_role)
  VALUES (NEW.updated_audit_event_id, NEW.book_id, 'transaction', NEW.id, 'primary');
  INSERT INTO audit_event_changes (audit_event_id, book_id, table_name, row_id, operation, before_json, after_json, changed_fields_json)
  VALUES (
    NEW.updated_audit_event_id,
    NEW.book_id,
    'transactions',
    NEW.id,
    'update',
    json_object('id', OLD.id, 'book_id', OLD.book_id, 'status', OLD.status, 'transaction_kind', OLD.transaction_kind, 'transaction_date', OLD.transaction_date, 'payee_id', OLD.payee_id, 'payee_name', OLD.payee_name, 'description', OLD.description),
    json_object('id', NEW.id, 'book_id', NEW.book_id, 'status', NEW.status, 'transaction_kind', NEW.transaction_kind, 'transaction_date', NEW.transaction_date, 'payee_id', NEW.payee_id, 'payee_name', NEW.payee_name, 'description', NEW.description),
    json_array('status', 'transaction_kind', 'transaction_date', 'payee_id', 'payee_name', 'description', 'external_ref_hint', 'note_markdown', 'metadata_json')
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
CREATE TRIGGER IF NOT EXISTS accounts_parent_same_book
BEFORE INSERT ON accounts
WHEN NEW.parent_account_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM accounts parent
    WHERE parent.id = NEW.parent_account_id
      AND parent.book_id = NEW.book_id
  )
BEGIN
  SELECT RAISE(ABORT, 'parent account must belong to the same book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS accounts_parent_same_book_update
BEFORE UPDATE OF parent_account_id ON accounts
WHEN NEW.parent_account_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM accounts parent
    WHERE parent.id = NEW.parent_account_id
      AND parent.book_id = NEW.book_id
  )
BEGIN
  SELECT RAISE(ABORT, 'parent account must belong to the same book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS postings_same_book_and_lineage
BEFORE INSERT ON postings
WHEN NOT EXISTS (
  SELECT 1
  FROM transactions t
  JOIN journal_entries je ON je.id = NEW.journal_entry_id
  JOIN posting_lines pl ON pl.id = NEW.posting_line_id
  JOIN accounts a ON a.id = NEW.account_id
  JOIN commodities c ON c.id = NEW.commodity_id
  WHERE t.id = NEW.transaction_id
    AND t.book_id = NEW.book_id
    AND je.transaction_id = t.id
    AND je.book_id = NEW.book_id
    AND pl.book_id = NEW.book_id
    AND pl.transaction_id = t.id
    AND a.book_id = NEW.book_id
    AND c.book_id = NEW.book_id
)
BEGIN
  SELECT RAISE(ABORT, 'posting references must belong to one transaction book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS postings_account_valid
BEFORE INSERT ON postings
WHEN NOT EXISTS (
  SELECT 1
  FROM journal_entries je
  JOIN accounts a ON a.id = NEW.account_id
  WHERE je.id = NEW.journal_entry_id
    AND a.status = 'active'
    AND a.allows_postings = 1
    AND je.entry_date >= a.opened_on
    AND (a.closed_on IS NULL OR je.entry_date <= a.closed_on)
    AND (a.default_commodity_id IS NULL OR a.default_commodity_id = NEW.commodity_id)
    AND (a.quantity_scale_override IS NULL OR NEW.quantity_scale <= a.quantity_scale_override)
)
BEGIN
  SELECT RAISE(ABORT, 'posting account is not eligible on entry date');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS postings_commodity_scale_valid
BEFORE INSERT ON postings
WHEN NOT EXISTS (
  SELECT 1
  FROM commodities c
  WHERE c.id = NEW.commodity_id
    AND c.status = 'active'
    AND NEW.quantity_scale <= c.max_quantity_scale
)
BEGIN
  SELECT RAISE(ABORT, 'posting commodity scale is invalid');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS postings_same_book_and_lineage_update
BEFORE UPDATE OF journal_entry_id, posting_line_id, account_id, commodity_id ON postings
WHEN NOT EXISTS (
  SELECT 1
  FROM transactions t
  JOIN journal_entries je ON je.id = NEW.journal_entry_id
  JOIN posting_lines pl ON pl.id = NEW.posting_line_id
  JOIN accounts a ON a.id = NEW.account_id
  JOIN commodities c ON c.id = NEW.commodity_id
  WHERE t.id = NEW.transaction_id
    AND t.book_id = NEW.book_id
    AND je.transaction_id = t.id
    AND je.book_id = NEW.book_id
    AND pl.book_id = NEW.book_id
    AND pl.transaction_id = t.id
    AND a.book_id = NEW.book_id
    AND c.book_id = NEW.book_id
)
BEGIN
  SELECT RAISE(ABORT, 'posting references must belong to one transaction book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS postings_account_valid_update
BEFORE UPDATE OF journal_entry_id, account_id, commodity_id, quantity_scale ON postings
WHEN NOT EXISTS (
  SELECT 1
  FROM journal_entries je
  JOIN accounts a ON a.id = NEW.account_id
  WHERE je.id = NEW.journal_entry_id
    AND a.status = 'active'
    AND a.allows_postings = 1
    AND je.entry_date >= a.opened_on
    AND (a.closed_on IS NULL OR je.entry_date <= a.closed_on)
    AND (a.default_commodity_id IS NULL OR a.default_commodity_id = NEW.commodity_id)
    AND (a.quantity_scale_override IS NULL OR NEW.quantity_scale <= a.quantity_scale_override)
)
BEGIN
  SELECT RAISE(ABORT, 'posting account is not eligible on entry date');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS postings_commodity_scale_valid_update
BEFORE UPDATE OF commodity_id, quantity_scale ON postings
WHEN NOT EXISTS (
  SELECT 1
  FROM commodities c
  WHERE c.id = NEW.commodity_id
    AND c.status = 'active'
    AND NEW.quantity_scale <= c.max_quantity_scale
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
DROP TRIGGER IF EXISTS postings_commodity_scale_valid_update;
DROP TRIGGER IF EXISTS postings_account_valid_update;
DROP TRIGGER IF EXISTS postings_same_book_and_lineage_update;
DROP TRIGGER IF EXISTS postings_commodity_scale_valid;
DROP TRIGGER IF EXISTS postings_account_valid;
DROP TRIGGER IF EXISTS postings_same_book_and_lineage;
DROP TRIGGER IF EXISTS accounts_parent_same_book_update;
DROP TRIGGER IF EXISTS accounts_parent_same_book;
DROP TRIGGER IF EXISTS transactions_correction_same_book;
DROP TRIGGER IF EXISTS audit_transactions_update;
DROP TRIGGER IF EXISTS audit_transactions_insert;
DROP TRIGGER IF EXISTS audit_payees_update;
DROP TRIGGER IF EXISTS audit_payees_insert;
DROP TRIGGER IF EXISTS audit_commodities_update;
DROP TRIGGER IF EXISTS audit_commodities_insert;
DROP TRIGGER IF EXISTS audit_institutions_update;
DROP TRIGGER IF EXISTS audit_institutions_insert;
DROP TRIGGER IF EXISTS audit_accounts_update;
DROP TRIGGER IF EXISTS audit_accounts_insert;
DROP TRIGGER IF EXISTS transaction_search_delete;
DROP TRIGGER IF EXISTS transaction_search_update;
DROP TRIGGER IF EXISTS transaction_search_insert;
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
DROP INDEX IF EXISTS postings_journal_idx;
DROP INDEX IF EXISTS postings_account_idx;
DROP INDEX IF EXISTS postings_transaction_idx;
DROP TABLE IF EXISTS postings;
DROP INDEX IF EXISTS posting_lines_transaction_idx;
DROP TABLE IF EXISTS posting_lines;
DROP INDEX IF EXISTS journal_entries_book_date_idx;
DROP INDEX IF EXISTS journal_entries_transaction_idx;
DROP TABLE IF EXISTS journal_entries;
DROP TABLE IF EXISTS transaction_search;
DROP INDEX IF EXISTS transactions_payee_idx;
DROP INDEX IF EXISTS transactions_correction_idx;
DROP INDEX IF EXISTS transactions_book_date_idx;
DROP TABLE IF EXISTS transactions;
DROP INDEX IF EXISTS payees_active_normalized_name_idx;
DROP INDEX IF EXISTS payees_book_status_name_idx;
DROP TABLE IF EXISTS payees;
DROP INDEX IF EXISTS accounts_institution_idx;
DROP INDEX IF EXISTS accounts_parent_idx;
DROP INDEX IF EXISTS accounts_book_status_class_idx;
DROP INDEX IF EXISTS accounts_system_role_idx;
DROP TABLE IF EXISTS accounts;
DROP TABLE IF EXISTS account_kinds;
DROP INDEX IF EXISTS institutions_book_idx;
DROP TABLE IF EXISTS institutions;
DROP INDEX IF EXISTS commodities_book_kind_idx;
DROP TABLE IF EXISTS commodities;
DROP INDEX IF EXISTS tags_active_name_kind_idx;
DROP INDEX IF EXISTS tags_book_status_kind_idx;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS books;
DROP TABLE IF EXISTS login_throttles;
DROP TABLE IF EXISTS setup_steps;
DROP INDEX IF EXISTS audit_event_changes_row_idx;
DROP TABLE IF EXISTS audit_event_changes;
DROP INDEX IF EXISTS audit_event_targets_target_idx;
DROP TABLE IF EXISTS audit_event_targets;
DROP INDEX IF EXISTS audit_events_request_idx;
DROP INDEX IF EXISTS audit_events_actor_occurred_idx;
DROP INDEX IF EXISTS audit_events_book_occurred_idx;
DROP TABLE IF EXISTS audit_events;
DROP INDEX IF EXISTS auth_sessions_expires_revoked_idx;
DROP INDEX IF EXISTS auth_sessions_user_id_idx;
DROP TABLE IF EXISTS auth_sessions;
DROP INDEX IF EXISTS users_one_owner_idx;
DROP TABLE IF EXISTS users;
