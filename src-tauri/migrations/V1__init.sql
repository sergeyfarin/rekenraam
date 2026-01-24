-- V1: full schema (consolidated)
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS books (
  id                INTEGER PRIMARY KEY,
  name              TEXT NOT NULL,
  kind              TEXT NOT NULL CHECK (kind IN ('personal', 'business')),
  base_commodity_id INTEGER,
  created_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE(name)
);

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
  metadata   TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  UNIQUE(book_id, kind, symbol, name)
);

CREATE INDEX IF NOT EXISTS idx_commodities_book_kind ON commodities(book_id, kind);

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

CREATE TABLE IF NOT EXISTS payees (
  id         INTEGER PRIMARY KEY,
  book_id    INTEGER NOT NULL,
  name       TEXT NOT NULL,
  kind       TEXT NOT NULL DEFAULT 'payee' CHECK (kind IN ('payee','customer','vendor','employee','other')),
  metadata   TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  UNIQUE(book_id, name)
);

CREATE TABLE IF NOT EXISTS categories (
  id         INTEGER PRIMARY KEY,
  book_id    INTEGER NOT NULL,
  parent_id  INTEGER,
  name       TEXT NOT NULL,
  kind       TEXT NOT NULL DEFAULT 'expense' CHECK (kind IN ('income','expense','transfer','other')),
  color      TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(parent_id) REFERENCES categories(id) ON DELETE SET NULL,
  UNIQUE(book_id, parent_id, name)
);

CREATE INDEX IF NOT EXISTS idx_categories_book_parent ON categories(book_id, parent_id);

CREATE TABLE IF NOT EXISTS tags (
  id         INTEGER PRIMARY KEY,
  book_id    INTEGER NOT NULL,
  name       TEXT NOT NULL,
  color      TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  UNIQUE(book_id, name)
);

CREATE TABLE IF NOT EXISTS people (
  id         INTEGER PRIMARY KEY,
  book_id    INTEGER NOT NULL,
  name       TEXT NOT NULL,
  role       TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('member','owner','employee','other')),
  metadata   TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  UNIQUE(book_id, name)
);

CREATE TABLE IF NOT EXISTS projects (
  id         INTEGER PRIMARY KEY,
  book_id    INTEGER NOT NULL,
  name       TEXT NOT NULL,
  status     TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
  metadata   TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  UNIQUE(book_id, name)
);

CREATE TABLE IF NOT EXISTS accounts (
  id           INTEGER PRIMARY KEY,
  book_id      INTEGER NOT NULL,
  parent_id    INTEGER,
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
  institution  TEXT,
  number_last4 TEXT,
  is_closed    INTEGER NOT NULL DEFAULT 0,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(parent_id) REFERENCES accounts(id) ON DELETE SET NULL,
  FOREIGN KEY(commodity_id) REFERENCES commodities(id) ON DELETE RESTRICT,
  UNIQUE(book_id, parent_id, name)
);

CREATE INDEX IF NOT EXISTS idx_accounts_book_type ON accounts(book_id, type);

CREATE TABLE IF NOT EXISTS transactions (
  id          INTEGER PRIMARY KEY,
  book_id     INTEGER NOT NULL,
  txn_date    TEXT NOT NULL,
  payee_id    INTEGER,
  memo        TEXT,
  status      TEXT NOT NULL DEFAULT 'uncleared' CHECK (status IN ('uncleared','cleared','reconciled','void')),
  reference   TEXT,
  import_id   TEXT,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(payee_id) REFERENCES payees(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_tx_book_date ON transactions(book_id, txn_date);
CREATE INDEX IF NOT EXISTS idx_tx_book_payee ON transactions(book_id, payee_id);

CREATE TABLE IF NOT EXISTS splits (
  id            INTEGER PRIMARY KEY,
  tx_id         INTEGER NOT NULL,
  account_id    INTEGER NOT NULL,
  commodity_id  INTEGER NOT NULL,
  amount_minor  INTEGER NOT NULL,
  category_id   INTEGER,
  memo          TEXT,
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(tx_id) REFERENCES transactions(id) ON DELETE CASCADE,
  FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE RESTRICT,
  FOREIGN KEY(commodity_id) REFERENCES commodities(id) ON DELETE RESTRICT,
  FOREIGN KEY(category_id) REFERENCES categories(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_splits_tx ON splits(tx_id);
CREATE INDEX IF NOT EXISTS idx_splits_account ON splits(account_id);
CREATE INDEX IF NOT EXISTS idx_splits_category ON splits(category_id);

CREATE TABLE IF NOT EXISTS split_tags (
  split_id INTEGER NOT NULL,
  tag_id   INTEGER NOT NULL,
  PRIMARY KEY(split_id, tag_id),
  FOREIGN KEY(split_id) REFERENCES splits(id) ON DELETE CASCADE,
  FOREIGN KEY(tag_id) REFERENCES tags(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS split_people (
  split_id     INTEGER NOT NULL,
  person_id    INTEGER NOT NULL,
  share_bps    INTEGER,
  PRIMARY KEY(split_id, person_id),
  FOREIGN KEY(split_id) REFERENCES splits(id) ON DELETE CASCADE,
  FOREIGN KEY(person_id) REFERENCES people(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS split_projects (
  split_id   INTEGER NOT NULL,
  project_id INTEGER NOT NULL,
  share_bps  INTEGER,
  PRIMARY KEY(split_id, project_id),
  FOREIGN KEY(split_id) REFERENCES splits(id) ON DELETE CASCADE,
  FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS lots (
  id           INTEGER PRIMARY KEY,
  book_id      INTEGER NOT NULL,
  account_id   INTEGER NOT NULL,
  commodity_id INTEGER NOT NULL,
  opened_date  TEXT,
  closed_date  TEXT,
  notes        TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE,
  FOREIGN KEY(commodity_id) REFERENCES commodities(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS split_lot_allocations (
  split_id       INTEGER NOT NULL,
  lot_id         INTEGER NOT NULL,
  quantity_minor INTEGER NOT NULL,
  PRIMARY KEY(split_id, lot_id),
  FOREIGN KEY(split_id) REFERENCES splits(id) ON DELETE CASCADE,
  FOREIGN KEY(lot_id) REFERENCES lots(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS book_state (
  book_id     INTEGER PRIMARY KEY,
  change_seq  INTEGER NOT NULL DEFAULT 0,
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE
);

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

CREATE TABLE IF NOT EXISTS corporate_actions (
  id              INTEGER PRIMARY KEY,
  book_id         INTEGER NOT NULL,
  commodity_id    INTEGER NOT NULL,
  kind            TEXT NOT NULL CHECK (kind IN ('split', 'merge')),
  ratio_num       INTEGER NOT NULL,
  ratio_den       INTEGER NOT NULL,
  effective_date  TEXT NOT NULL,
  memo            TEXT,
  tx_id           INTEGER,
  created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(commodity_id) REFERENCES commodities(id) ON DELETE CASCADE,
  FOREIGN KEY(tx_id) REFERENCES transactions(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_corporate_actions_book ON corporate_actions(book_id, effective_date);
CREATE INDEX IF NOT EXISTS idx_corporate_actions_commodity ON corporate_actions(commodity_id, effective_date);

CREATE TABLE IF NOT EXISTS price_sources (
  id          INTEGER PRIMARY KEY,
  name        TEXT NOT NULL,
  kind        TEXT NOT NULL DEFAULT 'manual' CHECK (kind IN ('manual','provider')),
  provider    TEXT,
  base_url    TEXT,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE(name)
);

CREATE TABLE IF NOT EXISTS commodity_price_sources (
  id            INTEGER PRIMARY KEY,
  commodity_id  INTEGER NOT NULL,
  source_id     INTEGER NOT NULL,
  symbol        TEXT NOT NULL,
  name_override TEXT,
  is_primary    INTEGER NOT NULL DEFAULT 0,
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(commodity_id) REFERENCES commodities(id) ON DELETE CASCADE,
  FOREIGN KEY(source_id) REFERENCES price_sources(id) ON DELETE CASCADE,
  UNIQUE(commodity_id, source_id, symbol)
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

CREATE TRIGGER IF NOT EXISTS trg_bump_split_tags_ins
AFTER INSERT ON split_tags
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
CREATE TRIGGER IF NOT EXISTS trg_bump_split_tags_del
AFTER DELETE ON split_tags
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

CREATE TRIGGER IF NOT EXISTS trg_bump_split_people_ins
AFTER INSERT ON split_people
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
CREATE TRIGGER IF NOT EXISTS trg_bump_split_people_upd
AFTER UPDATE ON split_people
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
CREATE TRIGGER IF NOT EXISTS trg_bump_split_people_del
AFTER DELETE ON split_people
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

CREATE TRIGGER IF NOT EXISTS trg_bump_split_projects_ins
AFTER INSERT ON split_projects
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
CREATE TRIGGER IF NOT EXISTS trg_bump_split_projects_upd
AFTER UPDATE ON split_projects
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
CREATE TRIGGER IF NOT EXISTS trg_bump_split_projects_del
AFTER DELETE ON split_projects
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
INSERT OR IGNORE INTO books (id, name, kind, created_at, updated_at)
VALUES (1, 'Personal', 'personal', (strftime('%Y-%m-%dT%H:%M:%fZ','now')), (strftime('%Y-%m-%dT%H:%M:%fZ','now')));

INSERT OR IGNORE INTO commodities (book_id, kind, symbol, name, scale, created_at, updated_at)
VALUES (
  (SELECT id FROM books WHERE name='Personal'),
  'currency',
  'USD',
  'US Dollar',
  2,
  (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

UPDATE books
SET base_commodity_id = (
  SELECT id FROM commodities WHERE book_id = books.id AND symbol = 'USD' LIMIT 1
)
WHERE base_commodity_id IS NULL AND name = 'Personal';

INSERT OR IGNORE INTO accounts (book_id, type, name, commodity_id, created_at, updated_at)
VALUES (
  (SELECT id FROM books WHERE name='Personal'),
  'cash',
  'Cash',
  (SELECT id FROM commodities WHERE book_id = (SELECT id FROM books WHERE name='Personal') AND symbol = 'USD'),
  (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

INSERT OR IGNORE INTO accounts (book_id, type, name, commodity_id, created_at, updated_at)
VALUES (
  (SELECT id FROM books WHERE name='Personal'),
  'checking',
  'Checking Account',
  (SELECT id FROM commodities WHERE book_id = (SELECT id FROM books WHERE name='Personal') AND symbol = 'USD'),
  (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

INSERT OR IGNORE INTO categories (book_id, parent_id, name, kind, created_at, updated_at)
VALUES (
  (SELECT id FROM books WHERE name='Personal'),
  NULL,
  'Groceries',
  'expense',
  (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

INSERT OR IGNORE INTO categories (book_id, parent_id, name, kind, created_at, updated_at)
VALUES (
  (SELECT id FROM books WHERE name='Personal'),
  NULL,
  'Salary',
  'income',
  (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

INSERT OR IGNORE INTO book_state (book_id, change_seq, updated_at)
SELECT id, 0, (strftime('%Y-%m-%dT%H:%M:%fZ','now')) FROM books WHERE name = 'Personal';

INSERT OR IGNORE INTO price_sources (id, name, kind)
VALUES (1, 'Manual', 'manual');
