-- +goose Up
CREATE TABLE IF NOT EXISTS accounts (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  is_system INTEGER NOT NULL CHECK (is_system IN (0, 1)),
  system_role TEXT CHECK (
    system_role IS NULL OR system_role IN (
      'opening_balance',
      'imbalance_import',
      'retained_earnings',
      'uncategorized_income',
      'uncategorized_expense'
    )
  ),
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_request_id TEXT,
  CHECK ((is_system = 1 AND system_role IS NOT NULL) OR (is_system = 0 AND system_role IS NULL))
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
  UNIQUE (account_id, version_seq)
);

CREATE INDEX IF NOT EXISTS account_versions_account_seq_idx
  ON account_versions (account_id, version_seq DESC);

CREATE INDEX IF NOT EXISTS account_versions_parent_idx
  ON account_versions (parent_account_id);

CREATE INDEX IF NOT EXISTS account_versions_institution_idx
  ON account_versions (institution_id);

CREATE INDEX IF NOT EXISTS account_versions_default_commodity_idx
  ON account_versions (default_commodity_id);

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS accounts_system_fields_no_update
BEFORE UPDATE OF is_system, system_role ON accounts
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
DROP INDEX IF EXISTS account_versions_default_commodity_idx;
DROP INDEX IF EXISTS account_versions_institution_idx;
DROP INDEX IF EXISTS account_versions_parent_idx;
DROP INDEX IF EXISTS account_versions_account_seq_idx;
DROP TABLE IF EXISTS account_versions;
DROP INDEX IF EXISTS accounts_book_idx;
DROP INDEX IF EXISTS accounts_system_role_idx;
DROP TABLE IF EXISTS accounts;
