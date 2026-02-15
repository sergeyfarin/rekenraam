-- V1: full schema (consolidated)
-- Maintainer note:
-- Before editing this file in future, ask explicitly whether the change should be done
-- as (a) a new migration file or (b) by modifying V1 directly.
PRAGMA foreign_keys = ON;

-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY
);

-- mutability: immutable rows (append-only enforced)
CREATE TABLE IF NOT EXISTS books (
  id                INTEGER PRIMARY KEY,
  previous_book_id  INTEGER,
  session_id        TEXT,
  name              TEXT NOT NULL,
  kind              TEXT NOT NULL CHECK (kind IN ('personal', 'business')),
  base_commodity_id INTEGER,
  created_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(previous_book_id) REFERENCES books(id) ON DELETE SET NULL
);

-- mutability: immutable rows (append-only enforced)
CREATE TABLE IF NOT EXISTS commodities (
  id         INTEGER PRIMARY KEY,
  book_id    INTEGER NOT NULL,
  kind       TEXT NOT NULL CHECK (kind IN (
    'currency',
    'stock',
    'etf',
    'public_fund',
    'private_fund',
    'non_traded_fund',
    'crypto',
    'real_estate',
    'other'
  )),
  symbol     TEXT,
  name       TEXT NOT NULL,
  scale      INTEGER NOT NULL DEFAULT 2,
  is_active  INTEGER NOT NULL DEFAULT 1,
  is_default INTEGER NOT NULL DEFAULT 0,
  display_symbol TEXT,
  metadata   TEXT,
  previous_commodity_id INTEGER,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(previous_commodity_id) REFERENCES commodities(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_commodities_book_kind ON commodities(book_id, kind);
CREATE INDEX IF NOT EXISTS idx_commodities_previous ON commodities(previous_commodity_id);

CREATE VIEW IF NOT EXISTS current_commodities AS
SELECT c.*
FROM commodities c
WHERE NOT EXISTS (
  SELECT 1 FROM commodities newer WHERE newer.previous_commodity_id = c.id
);

-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS commodity_prices (
  id                 INTEGER PRIMARY KEY,
  book_id            INTEGER NOT NULL,
  commodity_id       INTEGER NOT NULL,
  quote_commodity_id INTEGER NOT NULL,
  price_minor        INTEGER NOT NULL,
  as_of_date         TEXT NOT NULL,
  source             TEXT,
  source_id          INTEGER,
  is_manual          INTEGER NOT NULL DEFAULT 1,
  created_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(commodity_id) REFERENCES commodities(id) ON DELETE CASCADE,
  FOREIGN KEY(quote_commodity_id) REFERENCES commodities(id) ON DELETE CASCADE,
  UNIQUE(book_id, commodity_id, quote_commodity_id, as_of_date)
);

-- mutability: immutable rows (append-only enforced)
CREATE TABLE IF NOT EXISTS payees (
  id                INTEGER PRIMARY KEY,
  book_id           INTEGER NOT NULL,
  name              TEXT NOT NULL,
  kind              TEXT NOT NULL DEFAULT 'payee' CHECK (kind IN ('payee','customer','vendor','employee','other')),
  metadata          TEXT,
  previous_payee_id INTEGER,
  session_id        TEXT,
  created_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(previous_payee_id) REFERENCES payees(id) ON DELETE SET NULL
);

-- mutability: immutable rows (append-only enforced)
CREATE TABLE IF NOT EXISTS categories (
  id                   INTEGER PRIMARY KEY,
  book_id              INTEGER NOT NULL,
  parent_id            INTEGER,
  name                 TEXT NOT NULL,
  kind                 TEXT NOT NULL DEFAULT 'expense' CHECK (kind IN ('income','expense','transfer','other')),
  color                TEXT,
  previous_category_id INTEGER,
  session_id           TEXT,
  created_at           TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(parent_id) REFERENCES categories(id) ON DELETE SET NULL,
  FOREIGN KEY(previous_category_id) REFERENCES categories(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_categories_book_parent ON categories(book_id, parent_id);

-- mutability: immutable rows (append-only enforced)
CREATE TABLE IF NOT EXISTS tags (
  id               INTEGER PRIMARY KEY,
  book_id          INTEGER NOT NULL,
  name             TEXT NOT NULL,
  color            TEXT,
  previous_tag_id  INTEGER,
  session_id       TEXT,
  created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(previous_tag_id) REFERENCES tags(id) ON DELETE SET NULL
);

-- mutability: immutable rows (append-only enforced)
CREATE TABLE IF NOT EXISTS people (
  id         INTEGER PRIMARY KEY,
  book_id    INTEGER NOT NULL,
  name       TEXT NOT NULL,
  role       TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('member','owner','employee','other')),
  metadata   TEXT,
  previous_person_id INTEGER,
  session_id TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(previous_person_id) REFERENCES people(id) ON DELETE SET NULL
);

-- mutability: immutable rows (append-only enforced)
CREATE TABLE IF NOT EXISTS projects (
  id         INTEGER PRIMARY KEY,
  book_id    INTEGER NOT NULL,
  name       TEXT NOT NULL,
  status     TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
  metadata   TEXT,
  previous_project_id INTEGER,
  session_id TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(previous_project_id) REFERENCES projects(id) ON DELETE SET NULL
);

-- mutability: immutable rows (append-only enforced)
CREATE TABLE IF NOT EXISTS accounts (
  id           INTEGER PRIMARY KEY,
  book_id      INTEGER NOT NULL,
  parent_id    INTEGER,
  previous_account_id INTEGER,
  session_id   TEXT,
  type         TEXT NOT NULL CHECK (type IN (
    'cash', 'checking', 'savings',
    'credit', 'loan',
    'investment',
    'asset', 'liability',
    'income', 'expense',
    'equity'
  )),
  name         TEXT NOT NULL,
  commodity_id INTEGER NOT NULL,
  booking_policy TEXT NOT NULL DEFAULT 'fifo' CHECK (booking_policy IN ('fifo','lifo','strict','average')),
  institution  TEXT,
  number_last4 TEXT,
  institution_id INTEGER,
  country_id   INTEGER,
  is_hidden    INTEGER NOT NULL DEFAULT 0,
  is_system    INTEGER NOT NULL DEFAULT 0,
  system_role  TEXT,
  is_closed    INTEGER NOT NULL DEFAULT 0,
  effective_at TEXT NOT NULL DEFAULT (date('now')),
  lifecycle_event TEXT NOT NULL DEFAULT 'open' CHECK (lifecycle_event IN ('open','close','reopen','update')),
  lifecycle_note TEXT,
  lifecycle_metadata TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(parent_id) REFERENCES accounts(id) ON DELETE SET NULL,
  FOREIGN KEY(previous_account_id) REFERENCES accounts(id) ON DELETE SET NULL,
  FOREIGN KEY(commodity_id) REFERENCES commodities(id) ON DELETE RESTRICT,
  FOREIGN KEY(institution_id) REFERENCES institutions(id),
  FOREIGN KEY(country_id) REFERENCES countries(id)
);

CREATE INDEX IF NOT EXISTS idx_accounts_book_type ON accounts(book_id, type);
CREATE INDEX IF NOT EXISTS idx_accounts_book_effective ON accounts(book_id, effective_at);

CREATE TRIGGER IF NOT EXISTS trg_accounts_effective_date_format_ins
BEFORE INSERT ON accounts
BEGIN
  SELECT CASE
    WHEN NEW.effective_at NOT GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'
      OR date(NEW.effective_at) IS NULL
      THEN RAISE(ABORT, 'effective_at must be YYYY-MM-DD')
  END;

  SELECT CASE
    WHEN NEW.lifecycle_event = 'close' AND NEW.is_closed != 1
      THEN RAISE(ABORT, 'close lifecycle_event requires is_closed=1')
  END;

  SELECT CASE
    WHEN NEW.lifecycle_event IN ('open', 'reopen') AND NEW.is_closed != 0
      THEN RAISE(ABORT, 'open/reopen lifecycle_event requires is_closed=0')
  END;
END;

-- mutability: immutable rows (append-only enforced)
CREATE TABLE IF NOT EXISTS transactions (
  id          INTEGER PRIMARY KEY,
  book_id     INTEGER NOT NULL,
  previous_tx_id INTEGER,
  occurred_date    TEXT NOT NULL,
  occurred_at_utc TEXT,
  occurred_tz  TEXT,
  posted_date TEXT NOT NULL DEFAULT (date('now')),
  posted_at_utc TEXT,
  posted_tz TEXT,
  payee_id    INTEGER,
  memo        TEXT,
  status      TEXT NOT NULL DEFAULT 'uncleared' CHECK (status IN ('uncleared','cleared','reconciled','void')),
  reference   TEXT,
  import_id   TEXT,
  import_session_id INTEGER,
  session_id TEXT,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(previous_tx_id) REFERENCES transactions(id) ON DELETE SET NULL,
  FOREIGN KEY(payee_id) REFERENCES payees(id) ON DELETE SET NULL,
  FOREIGN KEY(import_session_id) REFERENCES import_sessions(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_tx_book_date ON transactions(book_id, occurred_date);
CREATE INDEX IF NOT EXISTS idx_tx_book_payee ON transactions(book_id, payee_id);
CREATE INDEX IF NOT EXISTS idx_tx_import_session ON transactions(import_session_id);

CREATE TRIGGER IF NOT EXISTS trg_transactions_datetime_format_ins
BEFORE INSERT ON transactions
BEGIN
  SELECT CASE
    WHEN NEW.occurred_date NOT GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'
      OR date(NEW.occurred_date) IS NULL
      THEN RAISE(ABORT, 'occurred_date must be YYYY-MM-DD')
  END;

  SELECT CASE
    WHEN NEW.occurred_at_utc IS NOT NULL AND (
      NEW.occurred_at_utc NOT GLOB '????-??-??T??:??*Z'
      OR datetime(NEW.occurred_at_utc) IS NULL
    )
      THEN RAISE(ABORT, 'occurred_at_utc must be UTC ISO-8601 (Z) when set')
  END;

  SELECT CASE
    WHEN NEW.occurred_at_utc IS NOT NULL AND (
      NEW.occurred_tz IS NULL
      OR trim(NEW.occurred_tz) = ''
      OR NEW.occurred_tz NOT GLOB '*/*'
    )
      THEN RAISE(ABORT, 'occurred_tz must be an IANA timezone (e.g. Area/City) when occurred_at_utc is set')
  END;

  SELECT CASE
    WHEN NEW.posted_date NOT GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'
      OR date(NEW.posted_date) IS NULL
      THEN RAISE(ABORT, 'posted_date must be YYYY-MM-DD')
  END;

  SELECT CASE
    WHEN NEW.posted_at_utc IS NOT NULL AND (
      NEW.posted_at_utc NOT GLOB '????-??-??T??:??*Z'
      OR datetime(NEW.posted_at_utc) IS NULL
    )
      THEN RAISE(ABORT, 'posted_at_utc must be UTC ISO-8601 (Z) when set')
  END;

  SELECT CASE
    WHEN NEW.posted_at_utc IS NOT NULL AND (
      NEW.posted_tz IS NULL
      OR trim(NEW.posted_tz) = ''
      OR NEW.posted_tz NOT GLOB '*/*'
    )
      THEN RAISE(ABORT, 'posted_tz must be an IANA timezone (e.g. Area/City) when posted_at_utc is set')
  END;

  SELECT CASE
    WHEN NEW.created_at NOT GLOB '????-??-??T??:??:??*Z'
      OR datetime(NEW.created_at) IS NULL
      THEN RAISE(ABORT, 'created_at must be UTC ISO-8601 (Z)')
  END;

END;

-- mutability: immutable rows (append-only enforced)
CREATE TABLE IF NOT EXISTS splits (
  id            INTEGER PRIMARY KEY,
  tx_id         INTEGER NOT NULL,
  previous_split_id INTEGER,
  session_id    TEXT,
  account_id    INTEGER NOT NULL,
  commodity_id  INTEGER NOT NULL,
  amount_minor  INTEGER NOT NULL,
  category_id   INTEGER,
  tag_id        INTEGER,
  person_id     INTEGER,
  project_id    INTEGER,
  share_bps     INTEGER,
  memo          TEXT,
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(tx_id) REFERENCES transactions(id) ON DELETE CASCADE,
  FOREIGN KEY(previous_split_id) REFERENCES splits(id) ON DELETE SET NULL,
  FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE RESTRICT,
  FOREIGN KEY(commodity_id) REFERENCES commodities(id) ON DELETE RESTRICT,
  FOREIGN KEY(category_id) REFERENCES categories(id) ON DELETE SET NULL,
  FOREIGN KEY(tag_id) REFERENCES tags(id) ON DELETE SET NULL,
  FOREIGN KEY(person_id) REFERENCES people(id) ON DELETE SET NULL,
  FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_splits_tx ON splits(tx_id);
CREATE INDEX IF NOT EXISTS idx_splits_account ON splits(account_id);
CREATE INDEX IF NOT EXISTS idx_splits_category ON splits(category_id);
CREATE INDEX IF NOT EXISTS idx_splits_tag ON splits(tag_id);
CREATE INDEX IF NOT EXISTS idx_splits_person ON splits(person_id);
CREATE INDEX IF NOT EXISTS idx_splits_project ON splits(project_id);

-- mutability: immutable rows (append-only enforced)
CREATE TABLE IF NOT EXISTS lots (
  id           INTEGER PRIMARY KEY,
  book_id      INTEGER NOT NULL,
  previous_lot_id INTEGER,
  session_id   TEXT,
  account_id   INTEGER NOT NULL,
  commodity_id INTEGER NOT NULL,
  cost_basis_minor INTEGER NOT NULL DEFAULT 0,
  opened_date  TEXT,
  closed_date  TEXT,
  notes        TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(previous_lot_id) REFERENCES lots(id) ON DELETE SET NULL,
  FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE,
  FOREIGN KEY(commodity_id) REFERENCES commodities(id) ON DELETE CASCADE
);

-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS split_lot_allocations (
  split_id       INTEGER NOT NULL,
  lot_id         INTEGER NOT NULL,
  quantity_minor INTEGER NOT NULL,
  PRIMARY KEY(split_id, lot_id),
  FOREIGN KEY(split_id) REFERENCES splits(id) ON DELETE CASCADE,
  FOREIGN KEY(lot_id) REFERENCES lots(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_lots_account_commodity_opened
  ON lots(account_id, commodity_id, opened_date, id);
CREATE INDEX IF NOT EXISTS idx_split_lot_allocations_lot_split
  ON split_lot_allocations(lot_id, split_id);

-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS book_state (
  book_id     INTEGER PRIMARY KEY,
  change_seq  INTEGER NOT NULL DEFAULT 0,
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE
);

-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS report_cache (
  id           INTEGER PRIMARY KEY,
  book_id      INTEGER NOT NULL,
  report_type  TEXT NOT NULL,
  params_hash  TEXT NOT NULL,
  params_json  TEXT,
  as_of_seq    INTEGER NOT NULL,
  payload_json TEXT NOT NULL,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  UNIQUE(book_id, report_type, params_hash, as_of_seq)
);

CREATE INDEX IF NOT EXISTS idx_report_cache_book_type ON report_cache(book_id, report_type);
CREATE INDEX IF NOT EXISTS idx_report_cache_book_seq ON report_cache(book_id, as_of_seq);

-- mutability: immutable rows (append-only enforced)
CREATE TABLE IF NOT EXISTS corporate_actions (
  id              INTEGER PRIMARY KEY,
  previous_corporate_action_id INTEGER,
  session_id      TEXT,
  book_id         INTEGER NOT NULL,
  commodity_id    INTEGER NOT NULL,
  kind            TEXT NOT NULL CHECK (kind IN ('split', 'merge')),
  ratio_num       INTEGER NOT NULL,
  ratio_den       INTEGER NOT NULL,
  effective_date  TEXT NOT NULL,
  memo            TEXT,
  tx_id           INTEGER,
  created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(previous_corporate_action_id) REFERENCES corporate_actions(id) ON DELETE SET NULL,
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(commodity_id) REFERENCES commodities(id) ON DELETE CASCADE,
  FOREIGN KEY(tx_id) REFERENCES transactions(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_corporate_actions_book ON corporate_actions(book_id, effective_date);
CREATE INDEX IF NOT EXISTS idx_corporate_actions_commodity ON corporate_actions(commodity_id, effective_date);

-- mutability: immutable rows (append-only enforced)
CREATE TABLE IF NOT EXISTS price_sources (
  id          INTEGER PRIMARY KEY,
  previous_price_source_id INTEGER,
  session_id  TEXT,
  name        TEXT NOT NULL,
  kind        TEXT NOT NULL DEFAULT 'manual' CHECK (kind IN ('manual','provider')),
  provider    TEXT,
  base_url    TEXT,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(previous_price_source_id) REFERENCES price_sources(id) ON DELETE SET NULL
);

-- mutability: immutable rows (append-only enforced)
CREATE TABLE IF NOT EXISTS commodity_price_sources (
  id            INTEGER PRIMARY KEY,
  previous_commodity_price_source_id INTEGER,
  session_id     TEXT,
  commodity_id  INTEGER NOT NULL,
  source_id     INTEGER NOT NULL,
  symbol        TEXT NOT NULL,
  name_override TEXT,
  is_primary    INTEGER NOT NULL DEFAULT 0,
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(previous_commodity_price_source_id) REFERENCES commodity_price_sources(id) ON DELETE SET NULL,
  FOREIGN KEY(commodity_id) REFERENCES commodities(id) ON DELETE CASCADE,
  FOREIGN KEY(source_id) REFERENCES price_sources(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_price_sources_kind ON price_sources(kind);
CREATE INDEX IF NOT EXISTS idx_cps_commodity_primary ON commodity_price_sources(commodity_id, is_primary);
CREATE INDEX IF NOT EXISTS idx_prices_source ON commodity_prices(source_id);

-- Invariants
CREATE TRIGGER IF NOT EXISTS trg_accounts_commodity_book_ins
BEFORE INSERT ON accounts
BEGIN
  SELECT
    CASE
      WHEN (SELECT book_id FROM commodities WHERE id = NEW.commodity_id) != NEW.book_id
      THEN RAISE(ABORT, 'account commodity must belong to same book')
    END;
END;

CREATE TRIGGER IF NOT EXISTS trg_accounts_commodity_book_upd
BEFORE UPDATE OF book_id, commodity_id ON accounts
BEGIN
  SELECT
    CASE
      WHEN (SELECT book_id FROM commodities WHERE id = NEW.commodity_id) != NEW.book_id
      THEN RAISE(ABORT, 'account commodity must belong to same book')
    END;
END;

CREATE TRIGGER IF NOT EXISTS trg_splits_commodity_matches_account_ins
BEFORE INSERT ON splits
BEGIN
  SELECT
    CASE
      WHEN NEW.commodity_id != (SELECT commodity_id FROM accounts WHERE id = NEW.account_id)
      THEN RAISE(ABORT, 'split commodity must match account commodity')
    END;
END;

CREATE TRIGGER IF NOT EXISTS trg_splits_commodity_matches_account_upd
BEFORE UPDATE OF account_id, commodity_id ON splits
BEGIN
  SELECT
    CASE
      WHEN NEW.commodity_id != (SELECT commodity_id FROM accounts WHERE id = NEW.account_id)
      THEN RAISE(ABORT, 'split commodity must match account commodity')
    END;
END;

CREATE TRIGGER IF NOT EXISTS trg_splits_book_matches_txn_ins
BEFORE INSERT ON splits
BEGIN
  SELECT
    CASE
      WHEN (SELECT book_id FROM transactions WHERE id = NEW.tx_id) != (SELECT book_id FROM accounts WHERE id = NEW.account_id)
      THEN RAISE(ABORT, 'split account must belong to same book as transaction')
    END;
END;

CREATE TRIGGER IF NOT EXISTS trg_splits_book_matches_txn_upd
BEFORE UPDATE OF tx_id, account_id ON splits
BEGIN
  SELECT
    CASE
      WHEN (SELECT book_id FROM transactions WHERE id = NEW.tx_id) != (SELECT book_id FROM accounts WHERE id = NEW.account_id)
      THEN RAISE(ABORT, 'split account must belong to same book as transaction')
    END;
END;

CREATE TRIGGER IF NOT EXISTS trg_splits_category_book_matches_txn_ins
BEFORE INSERT ON splits
WHEN NEW.category_id IS NOT NULL
BEGIN
  SELECT
    CASE
      WHEN (SELECT book_id FROM categories WHERE id = NEW.category_id) != (SELECT book_id FROM transactions WHERE id = NEW.tx_id)
      THEN RAISE(ABORT, 'split category must belong to same book as transaction')
    END;
END;

CREATE TRIGGER IF NOT EXISTS trg_splits_category_book_matches_txn_upd
BEFORE UPDATE OF tx_id, category_id ON splits
WHEN NEW.category_id IS NOT NULL
BEGIN
  SELECT
    CASE
      WHEN (SELECT book_id FROM categories WHERE id = NEW.category_id) != (SELECT book_id FROM transactions WHERE id = NEW.tx_id)
      THEN RAISE(ABORT, 'split category must belong to same book as transaction')
    END;
END;

CREATE TRIGGER IF NOT EXISTS trg_split_lot_allocations_match_split_lot_ins
BEFORE INSERT ON split_lot_allocations
BEGIN
  SELECT
    CASE
      WHEN (SELECT account_id FROM splits WHERE id = NEW.split_id) != (SELECT account_id FROM lots WHERE id = NEW.lot_id)
      THEN RAISE(ABORT, 'split_lot_allocation lot account must match split account')
    END;

  SELECT
    CASE
      WHEN (SELECT commodity_id FROM splits WHERE id = NEW.split_id) != (SELECT commodity_id FROM lots WHERE id = NEW.lot_id)
      THEN RAISE(ABORT, 'split_lot_allocation lot commodity must match split commodity')
    END;
END;

CREATE TRIGGER IF NOT EXISTS trg_split_lot_allocations_match_split_lot_upd
BEFORE UPDATE OF split_id, lot_id ON split_lot_allocations
BEGIN
  SELECT
    CASE
      WHEN (SELECT account_id FROM splits WHERE id = NEW.split_id) != (SELECT account_id FROM lots WHERE id = NEW.lot_id)
      THEN RAISE(ABORT, 'split_lot_allocation lot account must match split account')
    END;

  SELECT
    CASE
      WHEN (SELECT commodity_id FROM splits WHERE id = NEW.split_id) != (SELECT commodity_id FROM lots WHERE id = NEW.lot_id)
      THEN RAISE(ABORT, 'split_lot_allocation lot commodity must match split commodity')
    END;
END;

-- Precision bounds
CREATE TRIGGER IF NOT EXISTS trg_currency_scale_bounds_ins
BEFORE INSERT ON commodities
WHEN NEW.kind = 'currency' AND (NEW.scale < 0 OR NEW.scale > 9)
BEGIN
  SELECT RAISE(ABORT, 'currency scale must be between 0 and 9');
END;

CREATE TRIGGER IF NOT EXISTS trg_currency_scale_bounds_upd
BEFORE UPDATE OF kind, scale ON commodities
WHEN NEW.kind = 'currency' AND (NEW.scale < 0 OR NEW.scale > 9)
BEGIN
  SELECT RAISE(ABORT, 'currency scale must be between 0 and 9');
END;

CREATE TRIGGER IF NOT EXISTS trg_non_currency_scale_bounds_ins
BEFORE INSERT ON commodities
WHEN NEW.kind != 'currency' AND (NEW.scale < 0 OR NEW.scale > 9)
BEGIN
  SELECT RAISE(ABORT, 'commodity scale must be between 0 and 9');
END;

CREATE TRIGGER IF NOT EXISTS trg_non_currency_scale_bounds_upd
BEFORE UPDATE OF kind, scale ON commodities
WHEN NEW.kind != 'currency' AND (NEW.scale < 0 OR NEW.scale > 9)
BEGIN
  SELECT RAISE(ABORT, 'commodity scale must be between 0 and 9');
END;

-- Price source validity
CREATE TRIGGER IF NOT EXISTS trg_prices_source_valid
BEFORE INSERT ON commodity_prices
WHEN NEW.source_id IS NOT NULL
BEGIN
  SELECT
    CASE
      WHEN (SELECT id FROM price_sources WHERE id = NEW.source_id) IS NULL
      THEN RAISE(ABORT, 'price source not found')
    END;
END;

-- Book state + report cache triggers
CREATE TRIGGER IF NOT EXISTS trg_books_insert_state
AFTER INSERT ON books
BEGIN
  INSERT OR IGNORE INTO book_state (book_id) VALUES (NEW.id);
END;

CREATE TRIGGER IF NOT EXISTS trg_bump_accounts_ins
AFTER INSERT ON accounts
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_accounts_upd
AFTER UPDATE ON accounts
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_accounts_del
AFTER DELETE ON accounts
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = OLD.book_id;
END;

CREATE TRIGGER IF NOT EXISTS trg_bump_transactions_ins
AFTER INSERT ON transactions
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_transactions_upd
AFTER UPDATE ON transactions
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_transactions_del
AFTER DELETE ON transactions
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = OLD.book_id;
END;

CREATE TRIGGER IF NOT EXISTS trg_bump_categories_ins
AFTER INSERT ON categories
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_categories_upd
AFTER UPDATE ON categories
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_categories_del
AFTER DELETE ON categories
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = OLD.book_id;
END;

CREATE TRIGGER IF NOT EXISTS trg_bump_tags_ins
AFTER INSERT ON tags
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_tags_upd
AFTER UPDATE ON tags
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_tags_del
AFTER DELETE ON tags
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = OLD.book_id;
END;

CREATE TRIGGER IF NOT EXISTS trg_bump_payees_ins
AFTER INSERT ON payees
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_payees_upd
AFTER UPDATE ON payees
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_payees_del
AFTER DELETE ON payees
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = OLD.book_id;
END;

CREATE TRIGGER IF NOT EXISTS trg_bump_people_ins
AFTER INSERT ON people
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_people_upd
AFTER UPDATE ON people
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_people_del
AFTER DELETE ON people
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = OLD.book_id;
END;

CREATE TRIGGER IF NOT EXISTS trg_bump_projects_ins
AFTER INSERT ON projects
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_projects_upd
AFTER UPDATE ON projects
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_projects_del
AFTER DELETE ON projects
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = OLD.book_id;
END;

CREATE TRIGGER IF NOT EXISTS trg_bump_commodities_ins
AFTER INSERT ON commodities
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_commodities_upd
AFTER UPDATE ON commodities
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_commodities_del
AFTER DELETE ON commodities
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = OLD.book_id;
END;

CREATE TRIGGER IF NOT EXISTS trg_bump_prices_ins
AFTER INSERT ON commodity_prices
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_prices_upd
AFTER UPDATE ON commodity_prices
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_prices_del
AFTER DELETE ON commodity_prices
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = OLD.book_id;
END;

CREATE TRIGGER IF NOT EXISTS trg_bump_lots_ins
AFTER INSERT ON lots
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_lots_upd
AFTER UPDATE ON lots
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_lots_del
AFTER DELETE ON lots
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = OLD.book_id;
END;

CREATE TRIGGER IF NOT EXISTS trg_bump_splits_ins
AFTER INSERT ON splits
BEGIN
  UPDATE book_state
    SET change_seq = change_seq + 1,
        updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
    WHERE book_id = (SELECT book_id FROM transactions WHERE id = NEW.tx_id);
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_splits_upd
AFTER UPDATE ON splits
BEGIN
  UPDATE book_state
    SET change_seq = change_seq + 1,
        updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
    WHERE book_id = (SELECT book_id FROM transactions WHERE id = NEW.tx_id);
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_splits_del
AFTER DELETE ON splits
BEGIN
  UPDATE book_state
    SET change_seq = change_seq + 1,
        updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
    WHERE book_id = (SELECT book_id FROM transactions WHERE id = OLD.tx_id);
END;

CREATE TRIGGER IF NOT EXISTS trg_bump_split_lot_alloc_ins
AFTER INSERT ON split_lot_allocations
BEGIN
  UPDATE book_state
    SET change_seq = change_seq + 1,
        updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
    WHERE book_id = (
      SELECT t.book_id
      FROM splits s
      JOIN transactions t ON t.id = s.tx_id
      WHERE s.id = NEW.split_id
    );
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_split_lot_alloc_del
AFTER DELETE ON split_lot_allocations
BEGIN
  UPDATE book_state
    SET change_seq = change_seq + 1,
        updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
    WHERE book_id = (
      SELECT t.book_id
      FROM splits s
      JOIN transactions t ON t.id = s.tx_id
      WHERE s.id = OLD.split_id
    );
END;

-- Seed data
INSERT OR IGNORE INTO books (id, name, kind, created_at)
VALUES (1, 'Personal', 'personal', (strftime('%Y-%m-%dT%H:%M:%fZ','now')));

INSERT OR IGNORE INTO commodities (book_id, kind, symbol, name, scale, created_at)
VALUES (
  (SELECT id FROM books WHERE name='Personal'),
  'currency',
  'USD',
  'US Dollar',
  2,
  (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

INSERT OR IGNORE INTO categories (book_id, parent_id, name, kind, created_at)
VALUES (
  (SELECT id FROM books WHERE name='Personal'),
  NULL,
  'Groceries',
  'expense',
  (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

INSERT OR IGNORE INTO categories (book_id, parent_id, name, kind, created_at)
VALUES (
  (SELECT id FROM books WHERE name='Personal'),
  NULL,
  'Salary',
  'income',
  (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

INSERT OR IGNORE INTO book_state (book_id, change_seq, updated_at)
SELECT id, 0, (strftime('%Y-%m-%dT%H:%M:%fZ','now')) FROM books WHERE name = 'Personal';

INSERT OR IGNORE INTO price_sources (id, name, kind)
VALUES (1, 'Manual', 'manual');

-- Consolidated migrations V2-V17

-- V2: account balancing and locking
-- mutability: immutable rows (append-only enforced)
CREATE TABLE IF NOT EXISTS account_balancings (
  id            INTEGER PRIMARY KEY,
  previous_account_balancing_id INTEGER,
  session_id    TEXT,
  book_id       INTEGER NOT NULL,
  account_id    INTEGER NOT NULL,
  as_of_date    TEXT NOT NULL,
  balance_minor INTEGER NOT NULL,
  memo          TEXT,
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  voided_at     TEXT,
  void_reason   TEXT,
  FOREIGN KEY(previous_account_balancing_id) REFERENCES account_balancings(id) ON DELETE SET NULL,
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_account_balancings_book ON account_balancings(book_id);
CREATE INDEX IF NOT EXISTS idx_account_balancings_account_date ON account_balancings(account_id, as_of_date);
CREATE INDEX IF NOT EXISTS idx_account_balancings_active ON account_balancings(account_id, voided_at);

-- V3: dividend income categories
-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS dividend_income_categories (
  id            INTEGER PRIMARY KEY,
  book_id       INTEGER NOT NULL,
  category_id   INTEGER NOT NULL,
  commodity_id  INTEGER,
  tax_withheld_minor INTEGER,
  notes         TEXT,
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(category_id) REFERENCES categories(id) ON DELETE CASCADE,
  FOREIGN KEY(commodity_id) REFERENCES commodities(id) ON DELETE SET NULL,
  UNIQUE(book_id, category_id, commodity_id)
);

CREATE INDEX IF NOT EXISTS idx_dividend_income_categories_book ON dividend_income_categories(book_id);
CREATE INDEX IF NOT EXISTS idx_dividend_income_categories_category ON dividend_income_categories(category_id);

-- V4: documents/events/notes/pad/balance checks

-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS balance_checks (
  id            INTEGER PRIMARY KEY,
  book_id       INTEGER NOT NULL,
  account_id    INTEGER NOT NULL,
  as_of_date    TEXT NOT NULL,
  balance_minor INTEGER NOT NULL,
  memo          TEXT,
  status        TEXT NOT NULL DEFAULT 'recorded' CHECK (status IN ('recorded','matched','failed','unbalanced','balanced')),
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_balance_checks_account_date
  ON balance_checks(account_id, as_of_date);

-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS balance_adjustments (
  id                    INTEGER PRIMARY KEY,
  book_id               INTEGER NOT NULL,
  balance_check_id      INTEGER,
  account_id            INTEGER NOT NULL,
  offset_account_id     INTEGER NOT NULL,
  as_of_date            TEXT NOT NULL,
  target_balance_minor  INTEGER NOT NULL,
  actual_balance_minor  INTEGER NOT NULL,
  adjustment_minor      INTEGER NOT NULL,
  tx_id                 INTEGER NOT NULL,
  mark                  TEXT NOT NULL DEFAULT 'balance_adjustment',
  memo                  TEXT,
  created_at            TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(balance_check_id) REFERENCES balance_checks(id) ON DELETE SET NULL,
  FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE,
  FOREIGN KEY(offset_account_id) REFERENCES accounts(id) ON DELETE CASCADE,
  FOREIGN KEY(tx_id) REFERENCES transactions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_balance_adjustments_account_date
  ON balance_adjustments(account_id, as_of_date);

CREATE INDEX IF NOT EXISTS idx_balance_adjustments_check
  ON balance_adjustments(balance_check_id);

-- mutability: immutable rows (append-only enforced)
CREATE TABLE IF NOT EXISTS notes (
  id          INTEGER PRIMARY KEY,
  previous_note_id INTEGER,
  session_id  TEXT,
  book_id     INTEGER NOT NULL,
  account_id  INTEGER,
  tx_id       INTEGER,
  note        TEXT NOT NULL,
  note_date   TEXT,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(previous_note_id) REFERENCES notes(id) ON DELETE SET NULL,
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE SET NULL,
  FOREIGN KEY(tx_id) REFERENCES transactions(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_notes_account ON notes(account_id);
CREATE INDEX IF NOT EXISTS idx_notes_tx ON notes(tx_id);

-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS events (
  id          INTEGER PRIMARY KEY,
  book_id     INTEGER NOT NULL,
  account_id  INTEGER,
  tx_id       INTEGER,
  event_type  TEXT NOT NULL,
  event_date  TEXT NOT NULL,
  description TEXT,
  metadata    TEXT,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE SET NULL,
  FOREIGN KEY(tx_id) REFERENCES transactions(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_events_account_date
  ON events(account_id, event_date);

-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS documents (
  id          INTEGER PRIMARY KEY,
  book_id     INTEGER NOT NULL,
  account_id  INTEGER,
  tx_id       INTEGER,
  doc_type    TEXT,
  title       TEXT,
  uri         TEXT,
  mime_type   TEXT,
  notes       TEXT,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE SET NULL,
  FOREIGN KEY(tx_id) REFERENCES transactions(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_documents_account ON documents(account_id);
CREATE INDEX IF NOT EXISTS idx_documents_tx ON documents(tx_id);

CREATE TRIGGER IF NOT EXISTS trg_bump_balance_checks_ins
AFTER INSERT ON balance_checks
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_balance_checks_upd
AFTER UPDATE ON balance_checks
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_balance_checks_del
AFTER DELETE ON balance_checks
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = OLD.book_id;
END;

CREATE TRIGGER IF NOT EXISTS trg_bump_balance_adjustments_ins
AFTER INSERT ON balance_adjustments
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_balance_adjustments_upd
AFTER UPDATE ON balance_adjustments
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_balance_adjustments_del
AFTER DELETE ON balance_adjustments
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = OLD.book_id;
END;

CREATE TRIGGER IF NOT EXISTS trg_bump_notes_ins
AFTER INSERT ON notes
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_notes_upd
AFTER UPDATE ON notes
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_notes_del
AFTER DELETE ON notes
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = OLD.book_id;
END;

CREATE TRIGGER IF NOT EXISTS trg_bump_events_ins
AFTER INSERT ON events
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_events_upd
AFTER UPDATE ON events
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_events_del
AFTER DELETE ON events
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = OLD.book_id;
END;

CREATE TRIGGER IF NOT EXISTS trg_bump_documents_ins
AFTER INSERT ON documents
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_documents_upd
AFTER UPDATE ON documents
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_documents_del
AFTER DELETE ON documents
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = OLD.book_id;
END;

-- V5: booking policy + lot cost basis
CREATE INDEX IF NOT EXISTS idx_accounts_booking_policy ON accounts(booking_policy);

-- V6: balance constraints
-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS balance_constraints (
  id                INTEGER PRIMARY KEY,
  book_id           INTEGER NOT NULL,
  account_id        INTEGER NOT NULL,
  min_balance_minor INTEGER,
  max_balance_minor INTEGER,
  sign_rule         TEXT NOT NULL DEFAULT 'any' CHECK (sign_rule IN ('any','nonnegative','nonpositive')),
  created_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_balance_constraints_account ON balance_constraints(account_id);

CREATE TRIGGER IF NOT EXISTS trg_bump_balance_constraints_ins
AFTER INSERT ON balance_constraints
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_balance_constraints_upd
AFTER UPDATE ON balance_constraints
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_balance_constraints_del
AFTER DELETE ON balance_constraints
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = OLD.book_id;
END;

-- V7/V8: import rules + sessions (fully expanded)
-- mutability: immutable rows (append-only enforced)
CREATE TABLE IF NOT EXISTS import_rules (
  id                 INTEGER PRIMARY KEY,
  previous_import_rule_id INTEGER,
  session_id         TEXT,
  book_id            INTEGER NOT NULL,
  rule_kind          TEXT NOT NULL CHECK (rule_kind IN ('payee','memo','amount','date','account')),
  match_type         TEXT NOT NULL DEFAULT 'contains' CHECK (match_type IN ('contains','equals')),
  match_text         TEXT NOT NULL,
  priority           INTEGER NOT NULL DEFAULT 100,
  amount_min_minor   INTEGER,
  amount_max_minor   INTEGER,
  date_from          TEXT,
  date_to            TEXT,
  match_account_id   INTEGER,
  target_account_id  INTEGER,
  target_category_id INTEGER,
  target_payee_id    INTEGER,
  created_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(previous_import_rule_id) REFERENCES import_rules(id) ON DELETE SET NULL,
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(match_account_id) REFERENCES accounts(id) ON DELETE SET NULL,
  FOREIGN KEY(target_account_id) REFERENCES accounts(id) ON DELETE SET NULL,
  FOREIGN KEY(target_category_id) REFERENCES categories(id) ON DELETE SET NULL,
  FOREIGN KEY(target_payee_id) REFERENCES payees(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_import_rules_book_kind ON import_rules(book_id, rule_kind);
CREATE INDEX IF NOT EXISTS idx_import_rules_priority ON import_rules(priority, id);

-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS import_sessions (
  id         INTEGER PRIMARY KEY,
  book_id    INTEGER NOT NULL,
  source     TEXT,
  file_name  TEXT,
  file_hash  TEXT,
  file_size  INTEGER,
  status     TEXT NOT NULL DEFAULT 'started' CHECK (status IN ('started','committed','abandoned')),
  started_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  committed_at TEXT,
  notes      TEXT,
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE
);

-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS import_session_transactions (
  id          INTEGER PRIMARY KEY,
  session_id  INTEGER NOT NULL,
  tx_id       INTEGER NOT NULL,
  action      TEXT NOT NULL CHECK (action IN ('created','updated','validated')),
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(session_id) REFERENCES import_sessions(id) ON DELETE CASCADE,
  FOREIGN KEY(tx_id) REFERENCES transactions(id) ON DELETE CASCADE,
  UNIQUE(session_id, tx_id, action)
);

CREATE TRIGGER IF NOT EXISTS trg_import_sessions_datetime_format_ins
BEFORE INSERT ON import_sessions
BEGIN
  SELECT CASE
    WHEN NEW.started_at NOT GLOB '????-??-??T??:??:??*Z'
      OR datetime(NEW.started_at) IS NULL
      THEN RAISE(ABORT, 'started_at must be UTC ISO-8601 (Z)')
  END;

  SELECT CASE
    WHEN NEW.committed_at IS NOT NULL AND (
      NEW.committed_at NOT GLOB '????-??-??T??:??:??*Z'
      OR datetime(NEW.committed_at) IS NULL
    )
      THEN RAISE(ABORT, 'committed_at must be UTC ISO-8601 (Z) when set')
  END;
END;

CREATE TRIGGER IF NOT EXISTS trg_import_sessions_datetime_format_upd
BEFORE UPDATE OF started_at, committed_at ON import_sessions
BEGIN
  SELECT CASE
    WHEN NEW.started_at NOT GLOB '????-??-??T??:??:??*Z'
      OR datetime(NEW.started_at) IS NULL
      THEN RAISE(ABORT, 'started_at must be UTC ISO-8601 (Z)')
  END;

  SELECT CASE
    WHEN NEW.committed_at IS NOT NULL AND (
      NEW.committed_at NOT GLOB '????-??-??T??:??:??*Z'
      OR datetime(NEW.committed_at) IS NULL
    )
      THEN RAISE(ABORT, 'committed_at must be UTC ISO-8601 (Z) when set')
  END;
END;

CREATE TRIGGER IF NOT EXISTS trg_import_session_transactions_datetime_format_ins
BEFORE INSERT ON import_session_transactions
BEGIN
  SELECT CASE
    WHEN NEW.created_at NOT GLOB '????-??-??T??:??:??*Z'
      OR datetime(NEW.created_at) IS NULL
      THEN RAISE(ABORT, 'created_at must be UTC ISO-8601 (Z)')
  END;
END;

CREATE INDEX IF NOT EXISTS idx_import_session_transactions_session ON import_session_transactions(session_id);
CREATE INDEX IF NOT EXISTS idx_import_session_transactions_tx ON import_session_transactions(tx_id);

CREATE TRIGGER IF NOT EXISTS trg_bump_import_rules_ins
AFTER INSERT ON import_rules
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_import_rules_upd
AFTER UPDATE ON import_rules
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_import_rules_del
AFTER DELETE ON import_rules
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = OLD.book_id;
END;

-- V9: reporting
-- mutability: immutable rows (append-only enforced)
CREATE TABLE IF NOT EXISTS report_definitions (
  id            INTEGER PRIMARY KEY,
  previous_report_definition_id INTEGER,
  session_id    TEXT,
  book_id       INTEGER NOT NULL,
  name          TEXT NOT NULL,
  kind          TEXT NOT NULL DEFAULT 'custom' CHECK (kind IN ('builtin','custom')),
  query_type    TEXT NOT NULL DEFAULT 'sql' CHECK (query_type IN ('sql','template')),
  query_text    TEXT NOT NULL,
  params_schema TEXT,
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(previous_report_definition_id) REFERENCES report_definitions(id) ON DELETE SET NULL,
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_report_definitions_book ON report_definitions(book_id);
CREATE INDEX IF NOT EXISTS idx_report_definitions_kind ON report_definitions(book_id, kind);

-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS report_runs (
  id             INTEGER PRIMARY KEY,
  book_id        INTEGER NOT NULL,
  definition_id  INTEGER NOT NULL,
  params_hash    TEXT NOT NULL,
  as_of_seq      INTEGER NOT NULL,
  result_json    TEXT NOT NULL,
  created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(definition_id) REFERENCES report_definitions(id) ON DELETE CASCADE,
  UNIQUE(book_id, definition_id, params_hash, as_of_seq)
);

CREATE INDEX IF NOT EXISTS idx_report_runs_book_def ON report_runs(book_id, definition_id);
CREATE INDEX IF NOT EXISTS idx_report_runs_book_seq ON report_runs(book_id, as_of_seq);

-- V10: institutions + countries
-- mutability: immutable rows (append-only enforced)
CREATE TABLE IF NOT EXISTS countries (
  id         INTEGER PRIMARY KEY,
  previous_country_id INTEGER,
  session_id TEXT,
  book_id    INTEGER NOT NULL,
  code       TEXT NOT NULL,
  name       TEXT NOT NULL,
  default_currency_id INTEGER,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(previous_country_id) REFERENCES countries(id) ON DELETE SET NULL,
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(default_currency_id) REFERENCES currencies(id)
);

-- mutability: immutable rows (append-only enforced)
CREATE TABLE IF NOT EXISTS institutions (
  id         INTEGER PRIMARY KEY,
  previous_institution_id INTEGER,
  session_id TEXT,
  book_id    INTEGER NOT NULL,
  name       TEXT NOT NULL,
  kind       TEXT NOT NULL DEFAULT 'other' CHECK (kind IN ('bank','broker','credit_union','other')),
  country_id INTEGER,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(previous_institution_id) REFERENCES institutions(id) ON DELETE SET NULL,
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(country_id) REFERENCES countries(id) ON DELETE SET NULL
);

INSERT OR IGNORE INTO accounts (book_id, type, name, commodity_id, created_at)
VALUES (
  (SELECT id FROM books WHERE name='Personal'),
  'cash',
  'Cash',
  (SELECT id FROM commodities WHERE book_id = (SELECT id FROM books WHERE name='Personal') AND symbol = 'USD'),
  (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

INSERT OR IGNORE INTO accounts (book_id, type, name, commodity_id, created_at)
VALUES (
  (SELECT id FROM books WHERE name='Personal'),
  'checking',
  'Checking Account',
  (SELECT id FROM commodities WHERE book_id = (SELECT id FROM books WHERE name='Personal') AND symbol = 'USD'),
  (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- V18: hidden/system profit & loss accounts
CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_system_role_unique
  ON accounts(book_id, system_role)
  WHERE system_role IS NOT NULL;

CREATE TRIGGER IF NOT EXISTS trg_accounts_system_role_insert_guard
BEFORE INSERT ON accounts
WHEN NEW.system_role IS NOT NULL
  AND NEW.system_role NOT IN ('income_summary', 'expense_summary', 'retained_earnings')
BEGIN
  SELECT RAISE(ABORT, 'invalid accounts.system_role');
END;

CREATE TRIGGER IF NOT EXISTS trg_accounts_system_role_update_guard
BEFORE UPDATE ON accounts
WHEN NEW.system_role IS NOT NULL
  AND NEW.system_role NOT IN ('income_summary', 'expense_summary', 'retained_earnings')
BEGIN
  SELECT RAISE(ABORT, 'invalid accounts.system_role');
END;

CREATE TRIGGER IF NOT EXISTS trg_accounts_system_role_requires_system_insert
BEFORE INSERT ON accounts
WHEN NEW.system_role IS NOT NULL AND NEW.is_system = 0
BEGIN
  SELECT RAISE(ABORT, 'accounts.system_role requires is_system=1');
END;

CREATE TRIGGER IF NOT EXISTS trg_accounts_system_role_requires_system_update
BEFORE UPDATE ON accounts
WHEN NEW.system_role IS NOT NULL AND NEW.is_system = 0
BEGIN
  SELECT RAISE(ABORT, 'accounts.system_role requires is_system=1');
END;

INSERT OR IGNORE INTO accounts (
  book_id, type, name, commodity_id, is_hidden, is_system, system_role, created_at
)
VALUES (
  1,
  'income',
  'System Income Summary',
  (SELECT id FROM commodities WHERE book_id = 1 AND kind = 'currency' AND is_default = 1 LIMIT 1),
  1,
  1,
  'income_summary',
  (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

INSERT OR IGNORE INTO accounts (
  book_id, type, name, commodity_id, is_hidden, is_system, system_role, created_at
)
VALUES (
  1,
  'expense',
  'System Expense Summary',
  (SELECT id FROM commodities WHERE book_id = 1 AND kind = 'currency' AND is_default = 1 LIMIT 1),
  1,
  1,
  'expense_summary',
  (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

INSERT OR IGNORE INTO accounts (
  book_id, type, name, commodity_id, is_hidden, is_system, system_role, created_at
)
VALUES (
  1,
  'equity',
  'Retained Earnings',
  (SELECT id FROM commodities WHERE book_id = 1 AND kind = 'currency' AND is_default = 1 LIMIT 1),
  1,
  1,
  'retained_earnings',
  (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_countries_book ON countries(book_id);
CREATE INDEX IF NOT EXISTS idx_institutions_book ON institutions(book_id);
CREATE INDEX IF NOT EXISTS idx_accounts_institution ON accounts(institution_id);
CREATE INDEX IF NOT EXISTS idx_accounts_country ON accounts(country_id);
CREATE INDEX IF NOT EXISTS idx_accounts_book_hidden ON accounts(book_id, is_hidden);

-- V11: currencies seed
-- mutability: immutable rows (append-only enforced)
CREATE TABLE IF NOT EXISTS currencies (
  id         INTEGER PRIMARY KEY,
  previous_currency_id INTEGER,
  session_id TEXT,
  book_id    INTEGER NOT NULL,
  code       TEXT NOT NULL,
  name       TEXT NOT NULL,
  symbol     TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(previous_currency_id) REFERENCES currencies(id) ON DELETE SET NULL,
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_currencies_book ON currencies(book_id);
CREATE INDEX IF NOT EXISTS idx_countries_currency ON countries(default_currency_id);

INSERT OR IGNORE INTO currencies (book_id, code, name, symbol) VALUES
  (1, 'AED', 'United Arab Emirates Dirham', 'د.إ'),
  (1, 'AFN', 'Afghan Afghani', '؋'),
  (1, 'ALL', 'Albanian Lek', 'L'),
  (1, 'AMD', 'Armenian Dram', '֏'),
  (1, 'ANG', 'Netherlands Antillean Guilder', 'ƒ'),
  (1, 'AOA', 'Angolan Kwanza', 'Kz'),
  (1, 'ARS', 'Argentine Peso', '$'),
  (1, 'AUD', 'Australian Dollar', '$'),
  (1, 'AWG', 'Aruban Florin', 'ƒ'),
  (1, 'AZN', 'Azerbaijani Manat', '₼'),
  (1, 'BAM', 'Bosnia and Herzegovina Convertible Mark', 'KM'),
  (1, 'BBD', 'Barbados Dollar', '$'),
  (1, 'BDT', 'Bangladeshi Taka', '৳'),
  (1, 'BGN', 'Bulgarian Lev', 'лв'),
  (1, 'BHD', 'Bahraini Dinar', 'ب.د'),
  (1, 'BIF', 'Burundian Franc', 'Fr'),
  (1, 'BMD', 'Bermudian Dollar', '$'),
  (1, 'BND', 'Brunei Dollar', '$'),
  (1, 'BOB', 'Bolivian Boliviano', 'Bs.'),
  (1, 'BRL', 'Brazilian Real', 'R$'),
  (1, 'BSD', 'Bahamian Dollar', '$'),
  (1, 'BTN', 'Bhutanese Ngultrum', 'Nu.'),
  (1, 'BWP', 'Botswana Pula', 'P'),
  (1, 'BYN', 'Belarusian Ruble', 'Br'),
  (1, 'BZD', 'Belize Dollar', '$'),
  (1, 'CAD', 'Canadian Dollar', '$'),
  (1, 'CDF', 'Congolese Franc', 'Fr'),
  (1, 'CHF', 'Swiss Franc', 'Fr'),
  (1, 'CLP', 'Chilean Peso', '$'),
  (1, 'CNY', 'Chinese Yuan', '¥'),
  (1, 'COP', 'Colombian Peso', '$'),
  (1, 'CRC', 'Costa Rican Colón', '₡'),
  (1, 'CUP', 'Cuban Peso', '$'),
  (1, 'CVE', 'Cape Verdean Escudo', '$'),
  (1, 'CZK', 'Czech Koruna', 'Kč'),
  (1, 'DJF', 'Djiboutian Franc', 'Fr'),
  (1, 'DKK', 'Danish Krone', 'kr'),
  (1, 'DOP', 'Dominican Peso', '$'),
  (1, 'DZD', 'Algerian Dinar', 'دج'),
  (1, 'EGP', 'Egyptian Pound', '£'),
  (1, 'ERN', 'Eritrean Nakfa', 'Nfk'),
  (1, 'ETB', 'Ethiopian Birr', 'Br'),
  (1, 'EUR', 'Euro', '€'),
  (1, 'FJD', 'Fiji Dollar', '$'),
  (1, 'FKP', 'Falkland Islands Pound', '£'),
  (1, 'GBP', 'Pound Sterling', '£'),
  (1, 'GEL', 'Georgian Lari', '₾'),
  (1, 'GGP', 'Guernsey Pound', '£'),
  (1, 'GHS', 'Ghanaian Cedi', '₵'),
  (1, 'GIP', 'Gibraltar Pound', '£'),
  (1, 'GMD', 'Gambian Dalasi', 'D'),
  (1, 'GNF', 'Guinean Franc', 'Fr'),
  (1, 'GTQ', 'Guatemalan Quetzal', 'Q'),
  (1, 'GYD', 'Guyanese Dollar', '$'),
  (1, 'HKD', 'Hong Kong Dollar', '$'),
  (1, 'HNL', 'Honduran Lempira', 'L'),
  (1, 'HRK', 'Croatian Kuna', 'kn'),
  (1, 'HTG', 'Haitian Gourde', 'G'),
  (1, 'HUF', 'Hungarian Forint', 'Ft'),
  (1, 'IDR', 'Indonesian Rupiah', 'Rp'),
  (1, 'ILS', 'Israeli New Shekel', '₪'),
  (1, 'IMP', 'Manx Pound', '£'),
  (1, 'INR', 'Indian Rupee', '₹'),
  (1, 'IQD', 'Iraqi Dinar', 'ع.د'),
  (1, 'IRR', 'Iranian Rial', '﷼'),
  (1, 'ISK', 'Icelandic Króna', 'kr'),
  (1, 'JEP', 'Jersey Pound', '£'),
  (1, 'JMD', 'Jamaican Dollar', '$'),
  (1, 'JOD', 'Jordanian Dinar', 'د.ا'),
  (1, 'JPY', 'Japanese Yen', '¥'),
  (1, 'KES', 'Kenyan Shilling', 'Sh'),
  (1, 'KGS', 'Kyrgyzstani Som', 'с'),
  (1, 'KHR', 'Cambodian Riel', '៛'),
  (1, 'KID', 'Kiribati Dollar', '$'),
  (1, 'KMF', 'Comorian Franc', 'Fr'),
  (1, 'KPW', 'North Korean Won', '₩'),
  (1, 'KRW', 'South Korean Won', '₩'),
  (1, 'KWD', 'Kuwaiti Dinar', 'د.ك'),
  (1, 'KYD', 'Cayman Islands Dollar', '$'),
  (1, 'KZT', 'Kazakhstani Tenge', '₸'),
  (1, 'LAK', 'Lao Kip', '₭'),
  (1, 'LBP', 'Lebanese Pound', '£'),
  (1, 'LKR', 'Sri Lankan Rupee', 'Rs'),
  (1, 'LRD', 'Liberian Dollar', '$'),
  (1, 'LSL', 'Lesotho Loti', 'L'),
  (1, 'LYD', 'Libyan Dinar', 'ل.د'),
  (1, 'MAD', 'Moroccan Dirham', 'د.م.'),
  (1, 'MDL', 'Moldovan Leu', 'L'),
  (1, 'MGA', 'Malagasy Ariary', 'Ar'),
  (1, 'MKD', 'Macedonian Denar', 'ден'),
  (1, 'MMK', 'Myanmar Kyat', 'Ks'),
  (1, 'MNT', 'Mongolian Tögrög', '₮'),
  (1, 'MOP', 'Macanese Pataca', 'P'),
  (1, 'MRU', 'Mauritanian Ouguiya', 'UM'),
  (1, 'MUR', 'Mauritian Rupee', '₨'),
  (1, 'MVR', 'Maldivian Rufiyaa', 'Rf'),
  (1, 'MWK', 'Malawian Kwacha', 'MK'),
  (1, 'MXN', 'Mexican Peso', '$'),
  (1, 'MYR', 'Malaysian Ringgit', 'RM'),
  (1, 'MZN', 'Mozambican Metical', 'MT'),
  (1, 'NAD', 'Namibian Dollar', '$'),
  (1, 'NGN', 'Nigerian Naira', '₦'),
  (1, 'NIO', 'Nicaraguan Córdoba', 'C$'),
  (1, 'NOK', 'Norwegian Krone', 'kr'),
  (1, 'NPR', 'Nepalese Rupee', '₨'),
  (1, 'NZD', 'New Zealand Dollar', '$'),
  (1, 'OMR', 'Omani Rial', 'ر.ع.'),
  (1, 'PAB', 'Panamanian Balboa', 'B/.'),
  (1, 'PEN', 'Peruvian Sol', 'S/'),
  (1, 'PGK', 'Papua New Guinean Kina', 'K'),
  (1, 'PHP', 'Philippine Peso', '₱'),
  (1, 'PKR', 'Pakistani Rupee', '₨'),
  (1, 'PLN', 'Polish Złoty', 'zł'),
  (1, 'PYG', 'Paraguayan Guaraní', '₲'),
  (1, 'QAR', 'Qatari Riyal', 'ر.ق'),
  (1, 'RON', 'Romanian Leu', 'lei'),
  (1, 'RSD', 'Serbian Dinar', 'дин'),
  (1, 'RUB', 'Russian Ruble', '₽'),
  (1, 'RWF', 'Rwandan Franc', 'Fr'),
  (1, 'SAR', 'Saudi Riyal', 'ر.س'),
  (1, 'SBD', 'Solomon Islands Dollar', '$'),
  (1, 'SCR', 'Seychellois Rupee', '₨'),
  (1, 'SDG', 'Sudanese Pound', '£'),
  (1, 'SEK', 'Swedish Krona', 'kr'),
  (1, 'SGD', 'Singapore Dollar', '$'),
  (1, 'SHP', 'Saint Helena Pound', '£'),
  (1, 'SLL', 'Sierra Leonean Leone', 'Le'),
  (1, 'SOS', 'Somali Shilling', 'Sh'),
  (1, 'SRD', 'Surinamese Dollar', '$'),
  (1, 'SSP', 'South Sudanese Pound', '£'),
  (1, 'STN', 'São Tomé and Príncipe Dobra', 'Db'),
  (1, 'SYP', 'Syrian Pound', '£'),
  (1, 'SZL', 'Swazi Lilangeni', 'L'),
  (1, 'THB', 'Thai Baht', '฿'),
  (1, 'TJS', 'Tajikistani Somoni', 'ЅМ'),
  (1, 'TMT', 'Turkmenistan Manat', 'm'),
  (1, 'TND', 'Tunisian Dinar', 'د.ت'),
  (1, 'TOP', 'Tongan Paʻanga', 'T$'),
  (1, 'TRY', 'Turkish Lira', '₺'),
  (1, 'TTD', 'Trinidad and Tobago Dollar', '$'),
  (1, 'TVD', 'Tuvaluan Dollar', '$'),
  (1, 'TWD', 'New Taiwan Dollar', '$'),
  (1, 'TZS', 'Tanzanian Shilling', 'Sh'),
  (1, 'UAH', 'Ukrainian Hryvnia', '₴'),
  (1, 'UGX', 'Ugandan Shilling', 'Sh'),
  (1, 'USD', 'United States Dollar', '$'),
  (1, 'UYU', 'Uruguayan Peso', '$'),
  (1, 'UZS', 'Uzbekistani Som', 'soʻm'),
  (1, 'VES', 'Venezuelan Bolívar Soberano', 'Bs.S'),
  (1, 'VND', 'Vietnamese Đồng', '₫'),
  (1, 'VUV', 'Vanuatu Vatu', 'Vt'),
  (1, 'WST', 'Samoan Tālā', 'T'),
  (1, 'XAF', 'Central African CFA Franc', 'Fr'),
  (1, 'XCD', 'East Caribbean Dollar', '$'),
  (1, 'XOF', 'West African CFA Franc', 'Fr'),
  (1, 'XPF', 'CFP Franc', 'Fr'),
  (1, 'YER', 'Yemeni Rial', '﷼'),
  (1, 'ZAR', 'South African Rand', 'R'),
  (1, 'ZMW', 'Zambian Kwacha', 'ZK'),
  (1, 'ZWL', 'Zimbabwean Dollar', 'Z$');

INSERT OR IGNORE INTO countries (book_id, code, name, default_currency_id) VALUES
  (1, 'AF', 'Afghanistan', (SELECT id FROM currencies WHERE code='AFN' AND book_id=1)),
  (1, 'AX', 'Åland Islands', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'AL', 'Albania', (SELECT id FROM currencies WHERE code='ALL' AND book_id=1)),
  (1, 'DZ', 'Algeria', (SELECT id FROM currencies WHERE code='DZD' AND book_id=1)),
  (1, 'AS', 'American Samoa', (SELECT id FROM currencies WHERE code='USD' AND book_id=1)),
  (1, 'AD', 'Andorra', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'AO', 'Angola', (SELECT id FROM currencies WHERE code='AOA' AND book_id=1)),
  (1, 'AI', 'Anguilla', (SELECT id FROM currencies WHERE code='XCD' AND book_id=1)),
  (1, 'AQ', 'Antarctica', (SELECT id FROM currencies WHERE code='USD' AND book_id=1)),
  (1, 'AG', 'Antigua and Barbuda', (SELECT id FROM currencies WHERE code='XCD' AND book_id=1)),
  (1, 'AR', 'Argentina', (SELECT id FROM currencies WHERE code='ARS' AND book_id=1)),
  (1, 'AM', 'Armenia', (SELECT id FROM currencies WHERE code='AMD' AND book_id=1)),
  (1, 'AW', 'Aruba', (SELECT id FROM currencies WHERE code='AWG' AND book_id=1)),
  (1, 'AU', 'Australia', (SELECT id FROM currencies WHERE code='AUD' AND book_id=1)),
  (1, 'AT', 'Austria', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'AZ', 'Azerbaijan', (SELECT id FROM currencies WHERE code='AZN' AND book_id=1)),
  (1, 'BS', 'Bahamas', (SELECT id FROM currencies WHERE code='BSD' AND book_id=1)),
  (1, 'BH', 'Bahrain', (SELECT id FROM currencies WHERE code='BHD' AND book_id=1)),
  (1, 'BD', 'Bangladesh', (SELECT id FROM currencies WHERE code='BDT' AND book_id=1)),
  (1, 'BB', 'Barbados', (SELECT id FROM currencies WHERE code='BBD' AND book_id=1)),
  (1, 'BY', 'Belarus', (SELECT id FROM currencies WHERE code='BYN' AND book_id=1)),
  (1, 'BE', 'Belgium', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'BZ', 'Belize', (SELECT id FROM currencies WHERE code='BZD' AND book_id=1)),
  (1, 'BJ', 'Benin', (SELECT id FROM currencies WHERE code='XOF' AND book_id=1)),
  (1, 'BM', 'Bermuda', (SELECT id FROM currencies WHERE code='BMD' AND book_id=1)),
  (1, 'BT', 'Bhutan', (SELECT id FROM currencies WHERE code='BTN' AND book_id=1)),
  (1, 'BO', 'Bolivia', (SELECT id FROM currencies WHERE code='BOB' AND book_id=1)),
  (1, 'BQ', 'Bonaire, Sint Eustatius and Saba', (SELECT id FROM currencies WHERE code='USD' AND book_id=1)),
  (1, 'BA', 'Bosnia and Herzegovina', (SELECT id FROM currencies WHERE code='BAM' AND book_id=1)),
  (1, 'BW', 'Botswana', (SELECT id FROM currencies WHERE code='BWP' AND book_id=1)),
  (1, 'BV', 'Bouvet Island', (SELECT id FROM currencies WHERE code='NOK' AND book_id=1)),
  (1, 'BR', 'Brazil', (SELECT id FROM currencies WHERE code='BRL' AND book_id=1)),
  (1, 'IO', 'British Indian Ocean Territory', (SELECT id FROM currencies WHERE code='USD' AND book_id=1)),
  (1, 'BN', 'Brunei Darussalam', (SELECT id FROM currencies WHERE code='BND' AND book_id=1)),
  (1, 'BG', 'Bulgaria', (SELECT id FROM currencies WHERE code='BGN' AND book_id=1)),
  (1, 'BF', 'Burkina Faso', (SELECT id FROM currencies WHERE code='XOF' AND book_id=1)),
  (1, 'BI', 'Burundi', (SELECT id FROM currencies WHERE code='BIF' AND book_id=1)),
  (1, 'CV', 'Cabo Verde', (SELECT id FROM currencies WHERE code='CVE' AND book_id=1)),
  (1, 'KH', 'Cambodia', (SELECT id FROM currencies WHERE code='KHR' AND book_id=1)),
  (1, 'CM', 'Cameroon', (SELECT id FROM currencies WHERE code='XAF' AND book_id=1)),
  (1, 'CA', 'Canada', (SELECT id FROM currencies WHERE code='CAD' AND book_id=1)),
  (1, 'KY', 'Cayman Islands', (SELECT id FROM currencies WHERE code='KYD' AND book_id=1)),
  (1, 'CF', 'Central African Republic', (SELECT id FROM currencies WHERE code='XAF' AND book_id=1)),
  (1, 'TD', 'Chad', (SELECT id FROM currencies WHERE code='XAF' AND book_id=1)),
  (1, 'CL', 'Chile', (SELECT id FROM currencies WHERE code='CLP' AND book_id=1)),
  (1, 'CN', 'China', (SELECT id FROM currencies WHERE code='CNY' AND book_id=1)),
  (1, 'CX', 'Christmas Island', (SELECT id FROM currencies WHERE code='AUD' AND book_id=1)),
  (1, 'CC', 'Cocos (Keeling) Islands', (SELECT id FROM currencies WHERE code='AUD' AND book_id=1)),
  (1, 'CO', 'Colombia', (SELECT id FROM currencies WHERE code='COP' AND book_id=1)),
  (1, 'KM', 'Comoros', (SELECT id FROM currencies WHERE code='KMF' AND book_id=1)),
  (1, 'CD', 'Congo (Democratic Republic of the)', (SELECT id FROM currencies WHERE code='CDF' AND book_id=1)),
  (1, 'CG', 'Congo', (SELECT id FROM currencies WHERE code='XAF' AND book_id=1)),
  (1, 'CK', 'Cook Islands', (SELECT id FROM currencies WHERE code='NZD' AND book_id=1)),
  (1, 'CR', 'Costa Rica', (SELECT id FROM currencies WHERE code='CRC' AND book_id=1)),
  (1, 'CI', 'Côte d’Ivoire', (SELECT id FROM currencies WHERE code='XOF' AND book_id=1)),
  (1, 'HR', 'Croatia', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'CU', 'Cuba', (SELECT id FROM currencies WHERE code='CUP' AND book_id=1)),
  (1, 'CW', 'Curaçao', (SELECT id FROM currencies WHERE code='ANG' AND book_id=1)),
  (1, 'CY', 'Cyprus', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'CZ', 'Czechia', (SELECT id FROM currencies WHERE code='CZK' AND book_id=1)),
  (1, 'DK', 'Denmark', (SELECT id FROM currencies WHERE code='DKK' AND book_id=1)),
  (1, 'DJ', 'Djibouti', (SELECT id FROM currencies WHERE code='DJF' AND book_id=1)),
  (1, 'DM', 'Dominica', (SELECT id FROM currencies WHERE code='XCD' AND book_id=1)),
  (1, 'DO', 'Dominican Republic', (SELECT id FROM currencies WHERE code='DOP' AND book_id=1)),
  (1, 'EC', 'Ecuador', (SELECT id FROM currencies WHERE code='USD' AND book_id=1)),
  (1, 'EG', 'Egypt', (SELECT id FROM currencies WHERE code='EGP' AND book_id=1)),
  (1, 'SV', 'El Salvador', (SELECT id FROM currencies WHERE code='USD' AND book_id=1)),
  (1, 'GQ', 'Equatorial Guinea', (SELECT id FROM currencies WHERE code='XAF' AND book_id=1)),
  (1, 'ER', 'Eritrea', (SELECT id FROM currencies WHERE code='ERN' AND book_id=1)),
  (1, 'EE', 'Estonia', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'SZ', 'Eswatini', (SELECT id FROM currencies WHERE code='SZL' AND book_id=1)),
  (1, 'ET', 'Ethiopia', (SELECT id FROM currencies WHERE code='ETB' AND book_id=1)),
  (1, 'FK', 'Falkland Islands', (SELECT id FROM currencies WHERE code='FKP' AND book_id=1)),
  (1, 'FO', 'Faroe Islands', (SELECT id FROM currencies WHERE code='DKK' AND book_id=1)),
  (1, 'FJ', 'Fiji', (SELECT id FROM currencies WHERE code='FJD' AND book_id=1)),
  (1, 'FI', 'Finland', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'FR', 'France', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'GF', 'French Guiana', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'PF', 'French Polynesia', (SELECT id FROM currencies WHERE code='XPF' AND book_id=1)),
  (1, 'TF', 'French Southern Territories', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'GA', 'Gabon', (SELECT id FROM currencies WHERE code='XAF' AND book_id=1)),
  (1, 'GM', 'Gambia', (SELECT id FROM currencies WHERE code='GMD' AND book_id=1)),
  (1, 'GE', 'Georgia', (SELECT id FROM currencies WHERE code='GEL' AND book_id=1)),
  (1, 'DE', 'Germany', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'GH', 'Ghana', (SELECT id FROM currencies WHERE code='GHS' AND book_id=1)),
  (1, 'GI', 'Gibraltar', (SELECT id FROM currencies WHERE code='GIP' AND book_id=1)),
  (1, 'GR', 'Greece', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'GL', 'Greenland', (SELECT id FROM currencies WHERE code='DKK' AND book_id=1)),
  (1, 'GD', 'Grenada', (SELECT id FROM currencies WHERE code='XCD' AND book_id=1)),
  (1, 'GP', 'Guadeloupe', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'GU', 'Guam', (SELECT id FROM currencies WHERE code='USD' AND book_id=1)),
  (1, 'GT', 'Guatemala', (SELECT id FROM currencies WHERE code='GTQ' AND book_id=1)),
  (1, 'GG', 'Guernsey', (SELECT id FROM currencies WHERE code='GGP' AND book_id=1)),
  (1, 'GN', 'Guinea', (SELECT id FROM currencies WHERE code='GNF' AND book_id=1)),
  (1, 'GW', 'Guinea-Bissau', (SELECT id FROM currencies WHERE code='XOF' AND book_id=1)),
  (1, 'GY', 'Guyana', (SELECT id FROM currencies WHERE code='GYD' AND book_id=1)),
  (1, 'HT', 'Haiti', (SELECT id FROM currencies WHERE code='HTG' AND book_id=1)),
  (1, 'HM', 'Heard Island and McDonald Islands', (SELECT id FROM currencies WHERE code='AUD' AND book_id=1)),
  (1, 'VA', 'Holy See', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'HN', 'Honduras', (SELECT id FROM currencies WHERE code='HNL' AND book_id=1)),
  (1, 'HK', 'Hong Kong', (SELECT id FROM currencies WHERE code='HKD' AND book_id=1)),
  (1, 'HU', 'Hungary', (SELECT id FROM currencies WHERE code='HUF' AND book_id=1)),
  (1, 'IS', 'Iceland', (SELECT id FROM currencies WHERE code='ISK' AND book_id=1)),
  (1, 'IN', 'India', (SELECT id FROM currencies WHERE code='INR' AND book_id=1)),
  (1, 'ID', 'Indonesia', (SELECT id FROM currencies WHERE code='IDR' AND book_id=1)),
  (1, 'IR', 'Iran', (SELECT id FROM currencies WHERE code='IRR' AND book_id=1)),
  (1, 'IQ', 'Iraq', (SELECT id FROM currencies WHERE code='IQD' AND book_id=1)),
  (1, 'IE', 'Ireland', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'IM', 'Isle of Man', (SELECT id FROM currencies WHERE code='IMP' AND book_id=1)),
  (1, 'IL', 'Israel', (SELECT id FROM currencies WHERE code='ILS' AND book_id=1)),
  (1, 'IT', 'Italy', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'JM', 'Jamaica', (SELECT id FROM currencies WHERE code='JMD' AND book_id=1)),
  (1, 'JP', 'Japan', (SELECT id FROM currencies WHERE code='JPY' AND book_id=1)),
  (1, 'JE', 'Jersey', (SELECT id FROM currencies WHERE code='JEP' AND book_id=1)),
  (1, 'JO', 'Jordan', (SELECT id FROM currencies WHERE code='JOD' AND book_id=1)),
  (1, 'KZ', 'Kazakhstan', (SELECT id FROM currencies WHERE code='KZT' AND book_id=1)),
  (1, 'KE', 'Kenya', (SELECT id FROM currencies WHERE code='KES' AND book_id=1)),
  (1, 'KI', 'Kiribati', (SELECT id FROM currencies WHERE code='KID' AND book_id=1)),
  (1, 'KP', 'Korea (Democratic People''s Republic of)', (SELECT id FROM currencies WHERE code='KPW' AND book_id=1)),
  (1, 'KR', 'Korea (Republic of)', (SELECT id FROM currencies WHERE code='KRW' AND book_id=1)),
  (1, 'KW', 'Kuwait', (SELECT id FROM currencies WHERE code='KWD' AND book_id=1)),
  (1, 'KG', 'Kyrgyzstan', (SELECT id FROM currencies WHERE code='KGS' AND book_id=1)),
  (1, 'LA', 'Lao People''s Democratic Republic', (SELECT id FROM currencies WHERE code='LAK' AND book_id=1)),
  (1, 'LV', 'Latvia', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'LB', 'Lebanon', (SELECT id FROM currencies WHERE code='LBP' AND book_id=1)),
  (1, 'LS', 'Lesotho', (SELECT id FROM currencies WHERE code='LSL' AND book_id=1)),
  (1, 'LR', 'Liberia', (SELECT id FROM currencies WHERE code='LRD' AND book_id=1)),
  (1, 'LY', 'Libya', (SELECT id FROM currencies WHERE code='LYD' AND book_id=1)),
  (1, 'LI', 'Liechtenstein', (SELECT id FROM currencies WHERE code='CHF' AND book_id=1)),
  (1, 'LT', 'Lithuania', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'LU', 'Luxembourg', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'MO', 'Macao', (SELECT id FROM currencies WHERE code='MOP' AND book_id=1)),
  (1, 'MG', 'Madagascar', (SELECT id FROM currencies WHERE code='MGA' AND book_id=1)),
  (1, 'MW', 'Malawi', (SELECT id FROM currencies WHERE code='MWK' AND book_id=1)),
  (1, 'MY', 'Malaysia', (SELECT id FROM currencies WHERE code='MYR' AND book_id=1)),
  (1, 'MV', 'Maldives', (SELECT id FROM currencies WHERE code='MVR' AND book_id=1)),
  (1, 'ML', 'Mali', (SELECT id FROM currencies WHERE code='XOF' AND book_id=1)),
  (1, 'MT', 'Malta', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'MH', 'Marshall Islands', (SELECT id FROM currencies WHERE code='USD' AND book_id=1)),
  (1, 'MQ', 'Martinique', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'MR', 'Mauritania', (SELECT id FROM currencies WHERE code='MRU' AND book_id=1)),
  (1, 'MU', 'Mauritius', (SELECT id FROM currencies WHERE code='MUR' AND book_id=1)),
  (1, 'YT', 'Mayotte', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'MX', 'Mexico', (SELECT id FROM currencies WHERE code='MXN' AND book_id=1)),
  (1, 'FM', 'Micronesia (Federated States of)', (SELECT id FROM currencies WHERE code='USD' AND book_id=1)),
  (1, 'MD', 'Moldova', (SELECT id FROM currencies WHERE code='MDL' AND book_id=1)),
  (1, 'MC', 'Monaco', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'MN', 'Mongolia', (SELECT id FROM currencies WHERE code='MNT' AND book_id=1)),
  (1, 'ME', 'Montenegro', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'MS', 'Montserrat', (SELECT id FROM currencies WHERE code='XCD' AND book_id=1)),
  (1, 'MA', 'Morocco', (SELECT id FROM currencies WHERE code='MAD' AND book_id=1)),
  (1, 'MZ', 'Mozambique', (SELECT id FROM currencies WHERE code='MZN' AND book_id=1)),
  (1, 'MM', 'Myanmar', (SELECT id FROM currencies WHERE code='MMK' AND book_id=1)),
  (1, 'NA', 'Namibia', (SELECT id FROM currencies WHERE code='NAD' AND book_id=1)),
  (1, 'NR', 'Nauru', (SELECT id FROM currencies WHERE code='AUD' AND book_id=1)),
  (1, 'NP', 'Nepal', (SELECT id FROM currencies WHERE code='NPR' AND book_id=1)),
  (1, 'NL', 'Netherlands', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'NC', 'New Caledonia', (SELECT id FROM currencies WHERE code='XPF' AND book_id=1)),
  (1, 'NZ', 'New Zealand', (SELECT id FROM currencies WHERE code='NZD' AND book_id=1)),
  (1, 'NI', 'Nicaragua', (SELECT id FROM currencies WHERE code='NIO' AND book_id=1)),
  (1, 'NE', 'Niger', (SELECT id FROM currencies WHERE code='XOF' AND book_id=1)),
  (1, 'NG', 'Nigeria', (SELECT id FROM currencies WHERE code='NGN' AND book_id=1)),
  (1, 'NU', 'Niue', (SELECT id FROM currencies WHERE code='NZD' AND book_id=1)),
  (1, 'NF', 'Norfolk Island', (SELECT id FROM currencies WHERE code='AUD' AND book_id=1)),
  (1, 'MK', 'North Macedonia', (SELECT id FROM currencies WHERE code='MKD' AND book_id=1)),
  (1, 'MP', 'Northern Mariana Islands', (SELECT id FROM currencies WHERE code='USD' AND book_id=1)),
  (1, 'NO', 'Norway', (SELECT id FROM currencies WHERE code='NOK' AND book_id=1)),
  (1, 'OM', 'Oman', (SELECT id FROM currencies WHERE code='OMR' AND book_id=1)),
  (1, 'PK', 'Pakistan', (SELECT id FROM currencies WHERE code='PKR' AND book_id=1)),
  (1, 'PW', 'Palau', (SELECT id FROM currencies WHERE code='USD' AND book_id=1)),
  (1, 'PS', 'Palestine, State of', (SELECT id FROM currencies WHERE code='ILS' AND book_id=1)),
  (1, 'PA', 'Panama', (SELECT id FROM currencies WHERE code='PAB' AND book_id=1)),
  (1, 'PG', 'Papua New Guinea', (SELECT id FROM currencies WHERE code='PGK' AND book_id=1)),
  (1, 'PY', 'Paraguay', (SELECT id FROM currencies WHERE code='PYG' AND book_id=1)),
  (1, 'PE', 'Peru', (SELECT id FROM currencies WHERE code='PEN' AND book_id=1)),
  (1, 'PH', 'Philippines', (SELECT id FROM currencies WHERE code='PHP' AND book_id=1)),
  (1, 'PN', 'Pitcairn', (SELECT id FROM currencies WHERE code='NZD' AND book_id=1)),
  (1, 'PL', 'Poland', (SELECT id FROM currencies WHERE code='PLN' AND book_id=1)),
  (1, 'PT', 'Portugal', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'PR', 'Puerto Rico', (SELECT id FROM currencies WHERE code='USD' AND book_id=1)),
  (1, 'QA', 'Qatar', (SELECT id FROM currencies WHERE code='QAR' AND book_id=1)),
  (1, 'RE', 'Réunion', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'RO', 'Romania', (SELECT id FROM currencies WHERE code='RON' AND book_id=1)),
  (1, 'RU', 'Russian Federation', (SELECT id FROM currencies WHERE code='RUB' AND book_id=1)),
  (1, 'RW', 'Rwanda', (SELECT id FROM currencies WHERE code='RWF' AND book_id=1)),
  (1, 'BL', 'Saint Barthélemy', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'SH', 'Saint Helena, Ascension and Tristan da Cunha', (SELECT id FROM currencies WHERE code='SHP' AND book_id=1)),
  (1, 'KN', 'Saint Kitts and Nevis', (SELECT id FROM currencies WHERE code='XCD' AND book_id=1)),
  (1, 'LC', 'Saint Lucia', (SELECT id FROM currencies WHERE code='XCD' AND book_id=1)),
  (1, 'MF', 'Saint Martin (French part)', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'PM', 'Saint Pierre and Miquelon', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'VC', 'Saint Vincent and the Grenadines', (SELECT id FROM currencies WHERE code='XCD' AND book_id=1)),
  (1, 'WS', 'Samoa', (SELECT id FROM currencies WHERE code='WST' AND book_id=1)),
  (1, 'SM', 'San Marino', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'ST', 'Sao Tome and Principe', (SELECT id FROM currencies WHERE code='STN' AND book_id=1)),
  (1, 'SA', 'Saudi Arabia', (SELECT id FROM currencies WHERE code='SAR' AND book_id=1)),
  (1, 'SN', 'Senegal', (SELECT id FROM currencies WHERE code='XOF' AND book_id=1)),
  (1, 'RS', 'Serbia', (SELECT id FROM currencies WHERE code='RSD' AND book_id=1)),
  (1, 'SC', 'Seychelles', (SELECT id FROM currencies WHERE code='SCR' AND book_id=1)),
  (1, 'SL', 'Sierra Leone', (SELECT id FROM currencies WHERE code='SLL' AND book_id=1)),
  (1, 'SG', 'Singapore', (SELECT id FROM currencies WHERE code='SGD' AND book_id=1)),
  (1, 'SX', 'Sint Maarten (Dutch part)', (SELECT id FROM currencies WHERE code='ANG' AND book_id=1)),
  (1, 'SK', 'Slovakia', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'SI', 'Slovenia', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'SB', 'Solomon Islands', (SELECT id FROM currencies WHERE code='SBD' AND book_id=1)),
  (1, 'SO', 'Somalia', (SELECT id FROM currencies WHERE code='SOS' AND book_id=1)),
  (1, 'ZA', 'South Africa', (SELECT id FROM currencies WHERE code='ZAR' AND book_id=1)),
  (1, 'GS', 'South Georgia and the South Sandwich Islands', (SELECT id FROM currencies WHERE code='GBP' AND book_id=1)),
  (1, 'SS', 'South Sudan', (SELECT id FROM currencies WHERE code='SSP' AND book_id=1)),
  (1, 'ES', 'Spain', (SELECT id FROM currencies WHERE code='EUR' AND book_id=1)),
  (1, 'LK', 'Sri Lanka', (SELECT id FROM currencies WHERE code='LKR' AND book_id=1)),
  (1, 'SD', 'Sudan', (SELECT id FROM currencies WHERE code='SDG' AND book_id=1)),
  (1, 'SR', 'Suriname', (SELECT id FROM currencies WHERE code='SRD' AND book_id=1)),
  (1, 'SJ', 'Svalbard and Jan Mayen', (SELECT id FROM currencies WHERE code='NOK' AND book_id=1)),
  (1, 'SE', 'Sweden', (SELECT id FROM currencies WHERE code='SEK' AND book_id=1)),
  (1, 'CH', 'Switzerland', (SELECT id FROM currencies WHERE code='CHF' AND book_id=1)),
  (1, 'SY', 'Syrian Arab Republic', (SELECT id FROM currencies WHERE code='SYP' AND book_id=1)),
  (1, 'TW', 'Taiwan, Province of China', (SELECT id FROM currencies WHERE code='TWD' AND book_id=1)),
  (1, 'TJ', 'Tajikistan', (SELECT id FROM currencies WHERE code='TJS' AND book_id=1)),
  (1, 'TZ', 'Tanzania, United Republic of', (SELECT id FROM currencies WHERE code='TZS' AND book_id=1)),
  (1, 'TH', 'Thailand', (SELECT id FROM currencies WHERE code='THB' AND book_id=1)),
  (1, 'TL', 'Timor-Leste', (SELECT id FROM currencies WHERE code='USD' AND book_id=1)),
  (1, 'TG', 'Togo', (SELECT id FROM currencies WHERE code='XOF' AND book_id=1)),
  (1, 'TK', 'Tokelau', (SELECT id FROM currencies WHERE code='NZD' AND book_id=1)),
  (1, 'TO', 'Tonga', (SELECT id FROM currencies WHERE code='TOP' AND book_id=1)),
  (1, 'TT', 'Trinidad and Tobago', (SELECT id FROM currencies WHERE code='TTD' AND book_id=1)),
  (1, 'TN', 'Tunisia', (SELECT id FROM currencies WHERE code='TND' AND book_id=1)),
  (1, 'TR', 'Turkey', (SELECT id FROM currencies WHERE code='TRY' AND book_id=1)),
  (1, 'TM', 'Turkmenistan', (SELECT id FROM currencies WHERE code='TMT' AND book_id=1)),
  (1, 'TC', 'Turks and Caicos Islands', (SELECT id FROM currencies WHERE code='USD' AND book_id=1)),
  (1, 'TV', 'Tuvalu', (SELECT id FROM currencies WHERE code='TVD' AND book_id=1)),
  (1, 'UG', 'Uganda', (SELECT id FROM currencies WHERE code='UGX' AND book_id=1)),
  (1, 'UA', 'Ukraine', (SELECT id FROM currencies WHERE code='UAH' AND book_id=1)),
  (1, 'AE', 'United Arab Emirates', (SELECT id FROM currencies WHERE code='AED' AND book_id=1)),
  (1, 'GB', 'United Kingdom', (SELECT id FROM currencies WHERE code='GBP' AND book_id=1)),
  (1, 'US', 'United States of America', (SELECT id FROM currencies WHERE code='USD' AND book_id=1)),
  (1, 'UM', 'United States Minor Outlying Islands', (SELECT id FROM currencies WHERE code='USD' AND book_id=1)),
  (1, 'UY', 'Uruguay', (SELECT id FROM currencies WHERE code='UYU' AND book_id=1)),
  (1, 'UZ', 'Uzbekistan', (SELECT id FROM currencies WHERE code='UZS' AND book_id=1)),
  (1, 'VU', 'Vanuatu', (SELECT id FROM currencies WHERE code='VUV' AND book_id=1)),
  (1, 'VE', 'Venezuela (Bolivarian Republic of)', (SELECT id FROM currencies WHERE code='VES' AND book_id=1)),
  (1, 'VN', 'Viet Nam', (SELECT id FROM currencies WHERE code='VND' AND book_id=1)),
  (1, 'VG', 'Virgin Islands (British)', (SELECT id FROM currencies WHERE code='USD' AND book_id=1)),
  (1, 'VI', 'Virgin Islands (U.S.)', (SELECT id FROM currencies WHERE code='USD' AND book_id=1)),
  (1, 'WF', 'Wallis and Futuna', (SELECT id FROM currencies WHERE code='XPF' AND book_id=1)),
  (1, 'EH', 'Western Sahara', (SELECT id FROM currencies WHERE code='MAD' AND book_id=1)),
  (1, 'YE', 'Yemen', (SELECT id FROM currencies WHERE code='YER' AND book_id=1)),
  (1, 'ZM', 'Zambia', (SELECT id FROM currencies WHERE code='ZMW' AND book_id=1)),
  (1, 'ZW', 'Zimbabwe', (SELECT id FROM currencies WHERE code='ZWL' AND book_id=1));

-- V12: backup settings
-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS backup_settings (
  book_id          INTEGER PRIMARY KEY,
  enabled          INTEGER NOT NULL DEFAULT 0,
  interval_minutes INTEGER NOT NULL DEFAULT 60,
  retention_count  INTEGER NOT NULL DEFAULT 10,
  backup_path      TEXT,
  backup_on_close  INTEGER NOT NULL DEFAULT 1,
  created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE
);

INSERT OR IGNORE INTO backup_settings (book_id) VALUES (1);

-- V13: currency management and FX rates
CREATE INDEX IF NOT EXISTS idx_commodities_active ON commodities(book_id, kind, is_active);

-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS fx_rates_daily (
  id                 INTEGER PRIMARY KEY,
  book_id            INTEGER NOT NULL,
  from_currency_id   INTEGER NOT NULL,
  to_currency_id     INTEGER NOT NULL,
  rate_date          TEXT NOT NULL,
  rate               REAL NOT NULL,
  source             TEXT,
  source_id          INTEGER,
  is_derived         INTEGER NOT NULL DEFAULT 0,
  derived_via_currency_id INTEGER,
  created_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(from_currency_id) REFERENCES commodities(id) ON DELETE CASCADE,
  FOREIGN KEY(to_currency_id) REFERENCES commodities(id) ON DELETE CASCADE,
  FOREIGN KEY(source_id) REFERENCES fx_rate_sources(id) ON DELETE SET NULL,
  FOREIGN KEY(derived_via_currency_id) REFERENCES commodities(id) ON DELETE SET NULL,
  UNIQUE(book_id, from_currency_id, to_currency_id, rate_date)
);

CREATE INDEX IF NOT EXISTS idx_fx_daily_book_date ON fx_rates_daily(book_id, rate_date);
CREATE INDEX IF NOT EXISTS idx_fx_daily_currencies ON fx_rates_daily(from_currency_id, to_currency_id);

-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS fx_rates_official (
  id                 INTEGER PRIMARY KEY,
  book_id            INTEGER NOT NULL,
  from_currency_id   INTEGER NOT NULL,
  to_currency_id     INTEGER NOT NULL,
  period_type        TEXT NOT NULL CHECK (period_type IN ('monthly', 'yearly')),
  period_year        INTEGER NOT NULL,
  period_month       INTEGER,
  rate               REAL NOT NULL,
  source_name        TEXT NOT NULL,
  source_url         TEXT,
  source_date        TEXT,
  notes              TEXT,
  created_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(from_currency_id) REFERENCES commodities(id) ON DELETE CASCADE,
  FOREIGN KEY(to_currency_id) REFERENCES commodities(id) ON DELETE CASCADE,
  UNIQUE(book_id, from_currency_id, to_currency_id, period_type, period_year, period_month)
);

CREATE INDEX IF NOT EXISTS idx_fx_official_book ON fx_rates_official(book_id);
CREATE INDEX IF NOT EXISTS idx_fx_official_period ON fx_rates_official(period_type, period_year, period_month);
CREATE INDEX IF NOT EXISTS idx_fx_official_currencies ON fx_rates_official(from_currency_id, to_currency_id);

-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS fx_rate_sources (
  id           INTEGER PRIMARY KEY,
  book_id      INTEGER NOT NULL,
  name         TEXT NOT NULL,
  country_code TEXT,
  website_url  TEXT,
  notes        TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  UNIQUE(book_id, name)
);

INSERT OR IGNORE INTO fx_rate_sources (book_id, name, country_code, website_url, notes) VALUES
  (1, 'ECB', 'EU', 'https://www.ecb.europa.eu/stats/exchange/eurofxref/', 'European Central Bank reference rates'),
  (1, 'IRS', 'US', 'https://www.irs.gov/individuals/international-taxpayers/yearly-average-currency-exchange-rates', 'US Internal Revenue Service yearly average rates'),
  (1, 'HMRC', 'GB', 'https://www.gov.uk/government/collections/exchange-rates-for-customs-and-vat', 'UK HM Revenue & Customs rates'),
  (1, 'Belastingdienst', 'NL', 'https://www.belastingdienst.nl/wps/wcm/connect/nl/koerslijst/', 'Dutch Tax Authority rates'),
  (1, 'Federal Reserve', 'US', 'https://www.federalreserve.gov/releases/h10/current/', 'US Federal Reserve H.10 rates'),
  (1, 'Bank of Canada', 'CA', 'https://www.bankofcanada.ca/rates/exchange/', 'Bank of Canada exchange rates'),
  (1, 'RBA', 'AU', 'https://www.rba.gov.au/statistics/frequency/exchange-rates.html', 'Reserve Bank of Australia rates'),
  (1, 'SNB', 'CH', 'https://www.snb.ch/en/iabout/stat/statrep/id/current_interest_exchange_rates', 'Swiss National Bank rates');

UPDATE commodities
SET is_default = 1
WHERE book_id = 1 AND symbol = 'USD' AND kind = 'currency';

INSERT OR IGNORE INTO commodities (book_id, kind, symbol, name, scale, is_active, is_default, created_at) VALUES
  (1, 'currency', 'EUR', 'Euro', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'GBP', 'British Pound Sterling', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'JPY', 'Japanese Yen', 0, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'CHF', 'Swiss Franc', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'CAD', 'Canadian Dollar', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'AUD', 'Australian Dollar', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'CNY', 'Chinese Yuan', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'INR', 'Indian Rupee', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'MXN', 'Mexican Peso', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'BRL', 'Brazilian Real', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'KRW', 'South Korean Won', 0, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'SGD', 'Singapore Dollar', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'HKD', 'Hong Kong Dollar', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'NOK', 'Norwegian Krone', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'SEK', 'Swedish Krona', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'DKK', 'Danish Krone', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'NZD', 'New Zealand Dollar', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'ZAR', 'South African Rand', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'RUB', 'Russian Ruble', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'PLN', 'Polish Zloty', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'TRY', 'Turkish Lira', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'THB', 'Thai Baht', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'IDR', 'Indonesian Rupiah', 0, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'MYR', 'Malaysian Ringgit', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'PHP', 'Philippine Peso', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'CZK', 'Czech Koruna', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'ILS', 'Israeli New Shekel', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'AED', 'UAE Dirham', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'SAR', 'Saudi Riyal', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'TWD', 'Taiwan Dollar', 2, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'));

UPDATE commodities SET is_active = 1 WHERE book_id = 1 AND symbol = 'USD' AND kind = 'currency';

-- V14: currency display symbols
UPDATE commodities SET display_symbol = '$' WHERE symbol = 'USD' AND kind = 'currency';
UPDATE commodities SET display_symbol = '€' WHERE symbol = 'EUR' AND kind = 'currency';
UPDATE commodities SET display_symbol = '£' WHERE symbol = 'GBP' AND kind = 'currency';
UPDATE commodities SET display_symbol = '¥' WHERE symbol = 'JPY' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'Fr.' WHERE symbol = 'CHF' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'C$' WHERE symbol = 'CAD' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'A$' WHERE symbol = 'AUD' AND kind = 'currency';
UPDATE commodities SET display_symbol = '¥' WHERE symbol = 'CNY' AND kind = 'currency';
UPDATE commodities SET display_symbol = '₹' WHERE symbol = 'INR' AND kind = 'currency';
UPDATE commodities SET display_symbol = '$' WHERE symbol = 'MXN' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'R$' WHERE symbol = 'BRL' AND kind = 'currency';
UPDATE commodities SET display_symbol = '₩' WHERE symbol = 'KRW' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'S$' WHERE symbol = 'SGD' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'HK$' WHERE symbol = 'HKD' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'kr' WHERE symbol = 'NOK' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'kr' WHERE symbol = 'SEK' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'kr' WHERE symbol = 'DKK' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'NZ$' WHERE symbol = 'NZD' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'R' WHERE symbol = 'ZAR' AND kind = 'currency';
UPDATE commodities SET display_symbol = '₽' WHERE symbol = 'RUB' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'zł' WHERE symbol = 'PLN' AND kind = 'currency';
UPDATE commodities SET display_symbol = '₺' WHERE symbol = 'TRY' AND kind = 'currency';
UPDATE commodities SET display_symbol = '฿' WHERE symbol = 'THB' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'Rp' WHERE symbol = 'IDR' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'RM' WHERE symbol = 'MYR' AND kind = 'currency';
UPDATE commodities SET display_symbol = '₱' WHERE symbol = 'PHP' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'Kč' WHERE symbol = 'CZK' AND kind = 'currency';
UPDATE commodities SET display_symbol = '₪' WHERE symbol = 'ILS' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'د.إ' WHERE symbol = 'AED' AND kind = 'currency';
UPDATE commodities SET display_symbol = '﷼' WHERE symbol = 'SAR' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'NT$' WHERE symbol = 'TWD' AND kind = 'currency';

-- V15: FX rate refresh settings
-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS fx_rate_settings (
  book_id            INTEGER PRIMARY KEY,
  base_currency_id   INTEGER NOT NULL,
  default_source_id  INTEGER,
  refresh_enabled    INTEGER NOT NULL DEFAULT 1,
  refresh_hour_utc   INTEGER NOT NULL DEFAULT 4,
  refresh_minute_utc INTEGER NOT NULL DEFAULT 0,
  max_backfill_days  INTEGER NOT NULL DEFAULT 370,
  weekend_policy     TEXT NOT NULL DEFAULT 'skip' CHECK (weekend_policy IN ('skip', 'fill_previous', 'download')),
  created_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(base_currency_id) REFERENCES commodities(id) ON DELETE RESTRICT,
  FOREIGN KEY(default_source_id) REFERENCES fx_rate_sources(id) ON DELETE SET NULL
);

-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS fx_rate_source_assignments (
  id               INTEGER PRIMARY KEY,
  book_id          INTEGER NOT NULL,
  from_currency_id INTEGER NOT NULL,
  to_currency_id   INTEGER NOT NULL,
  source_id        INTEGER NOT NULL,
  effective_from   TEXT NOT NULL,
  effective_to     TEXT,
  created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(from_currency_id) REFERENCES commodities(id) ON DELETE CASCADE,
  FOREIGN KEY(to_currency_id) REFERENCES commodities(id) ON DELETE CASCADE,
  FOREIGN KEY(source_id) REFERENCES fx_rate_sources(id) ON DELETE CASCADE,
  UNIQUE(book_id, from_currency_id, to_currency_id, effective_from)
);

CREATE INDEX IF NOT EXISTS idx_fx_assignments_effective
  ON fx_rate_source_assignments(book_id, from_currency_id, to_currency_id, effective_from, effective_to);

-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS fx_rate_refresh_state (
  id               INTEGER PRIMARY KEY,
  book_id          INTEGER NOT NULL,
  from_currency_id INTEGER NOT NULL,
  to_currency_id   INTEGER NOT NULL,
  source_id        INTEGER NOT NULL,
  last_success_date TEXT,
  last_attempt_at   TEXT,
  last_error        TEXT,
  created_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(from_currency_id) REFERENCES commodities(id) ON DELETE CASCADE,
  FOREIGN KEY(to_currency_id) REFERENCES commodities(id) ON DELETE CASCADE,
  FOREIGN KEY(source_id) REFERENCES fx_rate_sources(id) ON DELETE CASCADE,
  UNIQUE(book_id, from_currency_id, to_currency_id, source_id)
);

CREATE INDEX IF NOT EXISTS idx_fx_refresh_state_pair
  ON fx_rate_refresh_state(book_id, from_currency_id, to_currency_id, source_id);

CREATE INDEX IF NOT EXISTS idx_fx_daily_source ON fx_rates_daily(source_id);

INSERT OR IGNORE INTO fx_rate_settings (book_id, base_currency_id, default_source_id)
SELECT 1,
       c.id,
       (
         SELECT id
         FROM fx_rate_sources
         WHERE book_id = 1 AND name IN ('ECB', 'Federal Reserve')
         ORDER BY CASE name WHEN 'ECB' THEN 0 ELSE 1 END
         LIMIT 1
       )
FROM commodities c
WHERE c.book_id = 1 AND c.kind = 'currency' AND c.is_default = 1
LIMIT 1;

-- V16: derived FX rates
CREATE INDEX IF NOT EXISTS idx_fx_daily_derived ON fx_rates_daily(is_derived);
CREATE INDEX IF NOT EXISTS idx_fx_daily_derived_via ON fx_rates_daily(derived_via_currency_id);

-- V17: commodities/currency sync
INSERT OR IGNORE INTO commodities
  (book_id, kind, symbol, display_symbol, name, scale, is_active, is_default, created_at)
SELECT
  c.book_id,
  'currency',
  c.code,
  c.symbol,
  c.name,
  2,
  CASE WHEN c.code IN ('USD', 'EUR', 'GBP') THEN 1 ELSE 0 END,
  CASE WHEN c.code = 'USD' THEN 1 ELSE 0 END,
  strftime('%Y-%m-%dT%H:%M:%fZ','now')
FROM currencies c
WHERE c.book_id = 1;

UPDATE commodities
SET is_active = 1
WHERE book_id = 1 AND kind = 'currency' AND symbol IN ('USD', 'EUR', 'GBP');

UPDATE commodities
SET is_default = 1, is_active = 1
WHERE book_id = 1 AND kind = 'currency' AND symbol = 'USD'
  AND NOT EXISTS (SELECT 1 FROM commodities WHERE book_id = 1 AND kind = 'currency' AND is_default = 1);

-- Append-only runtime/session metadata
-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS app_runtime_session (
  id         TEXT PRIMARY KEY,
  started_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS session_undo_stack (
  seq        INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  table_name TEXT NOT NULL,
  row_id     INTEGER NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS session_redo_stack (
  seq        INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  table_name TEXT NOT NULL,
  row_id     INTEGER NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- mutability: mutable rows
CREATE TABLE IF NOT EXISTS session_reverts (
  session_id  TEXT NOT NULL,
  table_name  TEXT NOT NULL,
  row_id      INTEGER NOT NULL,
  reverted_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY(session_id, table_name, row_id)
);

CREATE TRIGGER IF NOT EXISTS trg_app_runtime_session_datetime_format_ins
BEFORE INSERT ON app_runtime_session
BEGIN
  SELECT CASE
    WHEN NEW.started_at NOT GLOB '????-??-??T??:??:??*Z'
      OR datetime(NEW.started_at) IS NULL
      THEN RAISE(ABORT, 'started_at must be UTC ISO-8601 (Z)')
  END;
END;

CREATE TRIGGER IF NOT EXISTS trg_session_undo_stack_datetime_format_ins
BEFORE INSERT ON session_undo_stack
BEGIN
  SELECT CASE
    WHEN NEW.created_at NOT GLOB '????-??-??T??:??:??*Z'
      OR datetime(NEW.created_at) IS NULL
      THEN RAISE(ABORT, 'created_at must be UTC ISO-8601 (Z)')
  END;
END;

CREATE TRIGGER IF NOT EXISTS trg_session_redo_stack_datetime_format_ins
BEFORE INSERT ON session_redo_stack
BEGIN
  SELECT CASE
    WHEN NEW.created_at NOT GLOB '????-??-??T??:??:??*Z'
      OR datetime(NEW.created_at) IS NULL
      THEN RAISE(ABORT, 'created_at must be UTC ISO-8601 (Z)')
  END;
END;

CREATE TRIGGER IF NOT EXISTS trg_session_reverts_datetime_format_ins
BEFORE INSERT ON session_reverts
BEGIN
  SELECT CASE
    WHEN NEW.reverted_at NOT GLOB '????-??-??T??:??:??*Z'
      OR datetime(NEW.reverted_at) IS NULL
      THEN RAISE(ABORT, 'reverted_at must be UTC ISO-8601 (Z)')
  END;
END;

CREATE INDEX IF NOT EXISTS idx_transactions_previous_tx_id ON transactions(previous_tx_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_transactions_previous_unique
  ON transactions(previous_tx_id)
  WHERE previous_tx_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_payees_book_name ON payees(book_id, name);
CREATE INDEX IF NOT EXISTS idx_payees_previous ON payees(previous_payee_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_payees_previous_unique
  ON payees(previous_payee_id)
  WHERE previous_payee_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_categories_previous ON categories(previous_category_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_categories_previous_unique
  ON categories(previous_category_id)
  WHERE previous_category_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_tags_book_name ON tags(book_id, name);
CREATE INDEX IF NOT EXISTS idx_tags_previous ON tags(previous_tag_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tags_previous_unique
  ON tags(previous_tag_id)
  WHERE previous_tag_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_people_previous ON people(previous_person_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_people_previous_unique
  ON people(previous_person_id)
  WHERE previous_person_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_projects_previous ON projects(previous_project_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_previous_unique
  ON projects(previous_project_id)
  WHERE previous_project_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_accounts_previous ON accounts(previous_account_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_previous_unique
  ON accounts(previous_account_id)
  WHERE previous_account_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_lots_previous ON lots(previous_lot_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_lots_previous_unique
  ON lots(previous_lot_id)
  WHERE previous_lot_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_splits_previous ON splits(previous_split_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_splits_previous_unique
  ON splits(previous_split_id)
  WHERE previous_split_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_price_sources_previous ON price_sources(previous_price_source_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_price_sources_previous_unique
  ON price_sources(previous_price_source_id)
  WHERE previous_price_source_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_cps_previous ON commodity_price_sources(previous_commodity_price_source_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_cps_previous_unique
  ON commodity_price_sources(previous_commodity_price_source_id)
  WHERE previous_commodity_price_source_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_commodities_previous_unique
  ON commodities(previous_commodity_id)
  WHERE previous_commodity_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_books_previous ON books(previous_book_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_books_previous_unique
  ON books(previous_book_id)
  WHERE previous_book_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_account_balancings_previous ON account_balancings(previous_account_balancing_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_account_balancings_previous_unique
  ON account_balancings(previous_account_balancing_id)
  WHERE previous_account_balancing_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_notes_previous ON notes(previous_note_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_notes_previous_unique
  ON notes(previous_note_id)
  WHERE previous_note_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_import_rules_previous ON import_rules(previous_import_rule_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_import_rules_previous_unique
  ON import_rules(previous_import_rule_id)
  WHERE previous_import_rule_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_report_definitions_previous ON report_definitions(previous_report_definition_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_report_definitions_previous_unique
  ON report_definitions(previous_report_definition_id)
  WHERE previous_report_definition_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_countries_previous ON countries(previous_country_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_countries_previous_unique
  ON countries(previous_country_id)
  WHERE previous_country_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_institutions_previous ON institutions(previous_institution_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_institutions_previous_unique
  ON institutions(previous_institution_id)
  WHERE previous_institution_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_currencies_previous ON currencies(previous_currency_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_currencies_previous_unique
  ON currencies(previous_currency_id)
  WHERE previous_currency_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_corporate_actions_previous ON corporate_actions(previous_corporate_action_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_corporate_actions_previous_unique
  ON corporate_actions(previous_corporate_action_id)
  WHERE previous_corporate_action_id IS NOT NULL;

-- Immutability guards (enforce append-only behavior)

CREATE TRIGGER IF NOT EXISTS trg_transactions_append_only_update
BEFORE UPDATE ON transactions
BEGIN
  SELECT RAISE(ABORT, 'transactions are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_transactions_append_only_delete
BEFORE DELETE ON transactions
BEGIN
  SELECT RAISE(ABORT, 'transactions are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_payees_append_only_update
BEFORE UPDATE ON payees
BEGIN
  SELECT RAISE(ABORT, 'payees are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_payees_append_only_delete
BEFORE DELETE ON payees
BEGIN
  SELECT RAISE(ABORT, 'payees are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_categories_append_only_update
BEFORE UPDATE ON categories
BEGIN
  SELECT RAISE(ABORT, 'categories are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_categories_append_only_delete
BEFORE DELETE ON categories
BEGIN
  SELECT RAISE(ABORT, 'categories are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_tags_append_only_update
BEFORE UPDATE ON tags
BEGIN
  SELECT RAISE(ABORT, 'tags are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_tags_append_only_delete
BEFORE DELETE ON tags
BEGIN
  SELECT RAISE(ABORT, 'tags are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_commodities_append_only_update
BEFORE UPDATE ON commodities
BEGIN
  SELECT RAISE(ABORT, 'commodities are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_commodities_append_only_delete
BEFORE DELETE ON commodities
BEGIN
  SELECT RAISE(ABORT, 'commodities are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_people_append_only_update
BEFORE UPDATE ON people
BEGIN
  SELECT RAISE(ABORT, 'people are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_people_append_only_delete
BEFORE DELETE ON people
BEGIN
  SELECT RAISE(ABORT, 'people are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_projects_append_only_update
BEFORE UPDATE ON projects
BEGIN
  SELECT RAISE(ABORT, 'projects are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_projects_append_only_delete
BEFORE DELETE ON projects
BEGIN
  SELECT RAISE(ABORT, 'projects are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_accounts_append_only_update
BEFORE UPDATE ON accounts
BEGIN
  SELECT RAISE(ABORT, 'accounts are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_accounts_append_only_delete
BEFORE DELETE ON accounts
BEGIN
  SELECT RAISE(ABORT, 'accounts are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_lots_append_only_update
BEFORE UPDATE ON lots
BEGIN
  SELECT RAISE(ABORT, 'lots are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_lots_append_only_delete
BEFORE DELETE ON lots
BEGIN
  SELECT RAISE(ABORT, 'lots are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_splits_append_only_update
BEFORE UPDATE ON splits
BEGIN
  SELECT RAISE(ABORT, 'splits are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_splits_append_only_delete
BEFORE DELETE ON splits
BEGIN
  SELECT RAISE(ABORT, 'splits are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_price_sources_append_only_update
BEFORE UPDATE ON price_sources
BEGIN
  SELECT RAISE(ABORT, 'price_sources are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_price_sources_append_only_delete
BEFORE DELETE ON price_sources
BEGIN
  SELECT RAISE(ABORT, 'price_sources are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_commodity_price_sources_append_only_update
BEFORE UPDATE ON commodity_price_sources
BEGIN
  SELECT RAISE(ABORT, 'commodity_price_sources are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_commodity_price_sources_append_only_delete
BEFORE DELETE ON commodity_price_sources
BEGIN
  SELECT RAISE(ABORT, 'commodity_price_sources are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_books_append_only_update
BEFORE UPDATE ON books
BEGIN
  SELECT RAISE(ABORT, 'books are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_books_append_only_delete
BEFORE DELETE ON books
BEGIN
  SELECT RAISE(ABORT, 'books are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_account_balancings_append_only_update
BEFORE UPDATE ON account_balancings
BEGIN
  SELECT RAISE(ABORT, 'account_balancings are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_account_balancings_append_only_delete
BEFORE DELETE ON account_balancings
BEGIN
  SELECT RAISE(ABORT, 'account_balancings are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_currencies_append_only_update
BEFORE UPDATE ON currencies
BEGIN
  SELECT RAISE(ABORT, 'currencies are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_currencies_append_only_delete
BEFORE DELETE ON currencies
BEGIN
  SELECT RAISE(ABORT, 'currencies are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_corporate_actions_append_only_update
BEFORE UPDATE ON corporate_actions
BEGIN
  SELECT RAISE(ABORT, 'corporate_actions are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_corporate_actions_append_only_delete
BEFORE DELETE ON corporate_actions
BEGIN
  SELECT RAISE(ABORT, 'corporate_actions are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_countries_append_only_update
BEFORE UPDATE ON countries
BEGIN
  SELECT RAISE(ABORT, 'countries are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_countries_append_only_delete
BEFORE DELETE ON countries
BEGIN
  SELECT RAISE(ABORT, 'countries are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_institutions_append_only_update
BEFORE UPDATE ON institutions
BEGIN
  SELECT RAISE(ABORT, 'institutions are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_institutions_append_only_delete
BEFORE DELETE ON institutions
BEGIN
  SELECT RAISE(ABORT, 'institutions are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_report_definitions_append_only_update
BEFORE UPDATE ON report_definitions
BEGIN
  SELECT RAISE(ABORT, 'report_definitions are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_report_definitions_append_only_delete
BEFORE DELETE ON report_definitions
BEGIN
  SELECT RAISE(ABORT, 'report_definitions are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_import_rules_append_only_update
BEFORE UPDATE ON import_rules
BEGIN
  SELECT RAISE(ABORT, 'import_rules are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_import_rules_append_only_delete
BEFORE DELETE ON import_rules
BEGIN
  SELECT RAISE(ABORT, 'import_rules are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_notes_append_only_update
BEFORE UPDATE ON notes
BEGIN
  SELECT RAISE(ABORT, 'notes are append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_notes_append_only_delete
BEFORE DELETE ON notes
BEGIN
  SELECT RAISE(ABORT, 'notes are append-only');
END;

-- End append-only section
