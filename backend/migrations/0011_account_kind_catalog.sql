-- +goose Up
CREATE TABLE IF NOT EXISTS account_kinds (
  code TEXT PRIMARY KEY,
  account_class TEXT NOT NULL CHECK (account_class IN ('asset', 'liability', 'equity', 'income', 'expense')),
  base_kind TEXT NOT NULL,
  ui_profile TEXT NOT NULL,
  is_builtin INTEGER NOT NULL CHECK (is_builtin IN (0, 1)),
  is_user_assignable INTEGER NOT NULL CHECK (is_user_assignable IN (0, 1)),
  is_system_only INTEGER NOT NULL CHECK (is_system_only IN (0, 1)),
  display_key TEXT NOT NULL,
  sort_order INTEGER NOT NULL,
  UNIQUE (code, account_class)
);

INSERT OR IGNORE INTO account_kinds (
  code,
  account_class,
  base_kind,
  ui_profile,
  is_builtin,
  is_user_assignable,
  is_system_only,
  display_key,
  sort_order
) VALUES
  ('cash', 'asset', 'cash', 'cash', 1, 1, 0, 'account_kind_cash', 100),
  ('checking', 'asset', 'bank_account', 'checking', 1, 1, 0, 'account_kind_checking', 120),
  ('savings', 'asset', 'bank_account', 'savings', 1, 1, 0, 'account_kind_savings', 130),
  ('term_deposit', 'asset', 'bank_account', 'term_deposit', 1, 1, 0, 'account_kind_term_deposit', 140),
  ('brokerage', 'asset', 'investment_container', 'brokerage', 1, 1, 0, 'account_kind_brokerage', 200),
  ('brokerage_cash', 'asset', 'investment_cash', 'brokerage_cash', 1, 1, 0, 'account_kind_brokerage_cash', 210),
  ('security_holding', 'asset', 'security_holding', 'security_holding', 1, 1, 0, 'account_kind_security_holding', 220),
  ('crypto_wallet', 'asset', 'digital_asset', 'crypto_wallet', 1, 1, 0, 'account_kind_crypto_wallet', 230),
  ('property', 'asset', 'tangible_asset', 'property', 1, 1, 0, 'account_kind_property', 300),
  ('vehicle', 'asset', 'tangible_asset', 'vehicle', 1, 1, 0, 'account_kind_vehicle', 310),
  ('rewards_balance', 'asset', 'non_cash_balance', 'rewards_balance', 1, 1, 0, 'account_kind_rewards_balance', 400),
  ('receivable', 'asset', 'receivable', 'receivable', 1, 1, 0, 'account_kind_receivable', 500),
  ('other_asset', 'asset', 'other', 'other_asset', 1, 1, 0, 'account_kind_other_asset', 900),
  ('credit_card', 'liability', 'revolving_credit', 'credit_card', 1, 1, 0, 'account_kind_credit_card', 100),
  ('line_of_credit', 'liability', 'revolving_credit', 'line_of_credit', 1, 1, 0, 'account_kind_line_of_credit', 110),
  ('loan', 'liability', 'loan', 'loan', 1, 1, 0, 'account_kind_loan', 200),
  ('mortgage', 'liability', 'loan', 'mortgage', 1, 1, 0, 'account_kind_mortgage', 210),
  ('tax_liability', 'liability', 'payable', 'tax_liability', 1, 1, 0, 'account_kind_tax_liability', 300),
  ('payable', 'liability', 'payable', 'payable', 1, 1, 0, 'account_kind_payable', 310),
  ('other_liability', 'liability', 'other', 'other_liability', 1, 1, 0, 'account_kind_other_liability', 900),
  ('equity', 'equity', 'equity', 'equity', 1, 1, 0, 'account_kind_equity', 100),
  ('income', 'income', 'income', 'income', 1, 1, 1, 'account_kind_income', 100),
  ('expense', 'expense', 'expense', 'expense', 1, 1, 1, 'account_kind_expense', 100);

DROP TRIGGER IF EXISTS account_versions_no_delete;
DROP TRIGGER IF EXISTS account_versions_no_update;
DROP INDEX IF EXISTS account_versions_default_commodity_idx;
DROP INDEX IF EXISTS account_versions_institution_idx;
DROP INDEX IF EXISTS account_versions_parent_idx;
DROP INDEX IF EXISTS account_versions_account_seq_idx;

CREATE TABLE account_versions_new (
  id INTEGER PRIMARY KEY,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  version_seq INTEGER NOT NULL CHECK (version_seq > 0),
  effective_from TEXT NOT NULL,
  recorded_at TEXT NOT NULL,
  changed_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  change_reason TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('active', 'closed', 'archived')),
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
  FOREIGN KEY (account_kind, account_class) REFERENCES account_kinds(code, account_class) ON DELETE RESTRICT
);

INSERT INTO account_versions_new (
  id,
  account_id,
  version_seq,
  effective_from,
  recorded_at,
  changed_by_user_id,
  change_reason,
  status,
  code,
  name,
  account_class,
  account_kind,
  parent_account_id,
  institution_id,
  country_code,
  default_commodity_id,
  quantity_scale_override,
  allows_postings,
  number_last4,
  external_ref_hint,
  comment_markdown,
  metadata_json,
  change_audit_event_id
)
SELECT
  id,
  account_id,
  version_seq,
  effective_from,
  recorded_at,
  changed_by_user_id,
  change_reason,
  status,
  code,
  name,
  account_class,
  CASE
    WHEN account_kind = 'investment' THEN 'brokerage'
    WHEN account_kind = 'money_market' THEN 'savings'
    WHEN account_kind = 'time_deposit' THEN 'term_deposit'
    WHEN account_kind = 'collectible' THEN 'other_asset'
    WHEN account_kind = 'points_miles' THEN 'rewards_balance'
    WHEN account_kind = 'loan_receivable' THEN 'receivable'
    WHEN account_kind IN ('opening_balance', 'retained_earnings', 'current_earnings', 'trading', 'imbalance') THEN 'equity'
    WHEN account_kind IN ('salary', 'interest', 'dividend', 'realized_capital_gain', 'unrealized_capital_gain', 'reward_income', 'other_income') THEN 'income'
    WHEN account_kind IN ('fee', 'tax', 'interest_expense', 'investment_fee', 'other_expense', 'realized_losses', 'unrealized_losses') THEN 'expense'
    ELSE account_kind
  END,
  parent_account_id,
  institution_id,
  country_code,
  default_commodity_id,
  quantity_scale_override,
  allows_postings,
  number_last4,
  external_ref_hint,
  comment_markdown,
  metadata_json,
  change_audit_event_id
FROM account_versions;

DROP TABLE account_versions;
ALTER TABLE account_versions_new RENAME TO account_versions;

CREATE INDEX IF NOT EXISTS account_versions_account_seq_idx
  ON account_versions (account_id, version_seq DESC);

CREATE INDEX IF NOT EXISTS account_versions_parent_idx
  ON account_versions (parent_account_id);

CREATE INDEX IF NOT EXISTS account_versions_institution_idx
  ON account_versions (institution_id);

CREATE INDEX IF NOT EXISTS account_versions_default_commodity_idx
  ON account_versions (default_commodity_id);

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
DROP INDEX IF EXISTS account_versions_default_commodity_idx;
DROP INDEX IF EXISTS account_versions_institution_idx;
DROP INDEX IF EXISTS account_versions_parent_idx;
DROP INDEX IF EXISTS account_versions_account_seq_idx;

CREATE TABLE account_versions_old (
  id INTEGER PRIMARY KEY,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  version_seq INTEGER NOT NULL CHECK (version_seq > 0),
  effective_from TEXT NOT NULL,
  recorded_at TEXT NOT NULL,
  changed_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  change_reason TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('active', 'closed', 'archived')),
  code TEXT,
  name TEXT,
  account_class TEXT NOT NULL CHECK (account_class IN ('asset', 'liability', 'equity', 'income', 'expense')),
  account_kind TEXT NOT NULL CHECK (account_kind IN (
    'cash',
    'checking',
    'savings',
    'time_deposit',
    'money_market',
    'investment',
    'brokerage_cash',
    'security_holding',
    'crypto_wallet',
    'property',
    'vehicle',
    'collectible',
    'points_miles',
    'loan_receivable',
    'other_asset',
    'credit_card',
    'line_of_credit',
    'loan',
    'mortgage',
    'tax_liability',
    'payable',
    'other_liability',
    'opening_balance',
    'retained_earnings',
    'current_earnings',
    'trading',
    'imbalance',
    'equity',
    'salary',
    'interest',
    'dividend',
    'realized_capital_gain',
    'unrealized_capital_gain',
    'reward_income',
    'other_income',
    'expense',
    'fee',
    'tax',
    'interest_expense',
    'investment_fee',
    'other_expense',
    'realized_losses',
    'unrealized_losses'
  )),
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
  UNIQUE (account_id, version_seq)
);

INSERT INTO account_versions_old (
  id,
  account_id,
  version_seq,
  effective_from,
  recorded_at,
  changed_by_user_id,
  change_reason,
  status,
  code,
  name,
  account_class,
  account_kind,
  parent_account_id,
  institution_id,
  country_code,
  default_commodity_id,
  quantity_scale_override,
  allows_postings,
  number_last4,
  external_ref_hint,
  comment_markdown,
  metadata_json,
  change_audit_event_id
)
SELECT
  id,
  account_id,
  version_seq,
  effective_from,
  recorded_at,
  changed_by_user_id,
  change_reason,
  status,
  code,
  name,
  account_class,
  CASE
    WHEN account_kind = 'brokerage' THEN 'investment'
    WHEN account_kind = 'rewards_balance' THEN 'points_miles'
    WHEN account_kind = 'receivable' THEN 'loan_receivable'
    WHEN account_kind = 'term_deposit' THEN 'time_deposit'
    WHEN account_kind = 'income' THEN 'other_income'
    ELSE account_kind
  END,
  parent_account_id,
  institution_id,
  country_code,
  default_commodity_id,
  quantity_scale_override,
  allows_postings,
  number_last4,
  external_ref_hint,
  comment_markdown,
  metadata_json,
  change_audit_event_id
FROM account_versions;

DROP TABLE account_versions;
ALTER TABLE account_versions_old RENAME TO account_versions;
DROP TABLE IF EXISTS account_kinds;

CREATE INDEX IF NOT EXISTS account_versions_account_seq_idx
  ON account_versions (account_id, version_seq DESC);

CREATE INDEX IF NOT EXISTS account_versions_parent_idx
  ON account_versions (parent_account_id);

CREATE INDEX IF NOT EXISTS account_versions_institution_idx
  ON account_versions (institution_id);

CREATE INDEX IF NOT EXISTS account_versions_default_commodity_idx
  ON account_versions (default_commodity_id);

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
