-- V2: account balancing and locking
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS account_balancings (
  id            INTEGER PRIMARY KEY,
  book_id       INTEGER NOT NULL,
  account_id    INTEGER NOT NULL,
  as_of_date    TEXT NOT NULL,
  balance_minor INTEGER NOT NULL,
  memo          TEXT,
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  voided_at     TEXT,
  void_reason   TEXT,
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_account_balancings_book ON account_balancings(book_id);
CREATE INDEX IF NOT EXISTS idx_account_balancings_account_date ON account_balancings(account_id, as_of_date);
CREATE INDEX IF NOT EXISTS idx_account_balancings_active ON account_balancings(account_id, voided_at);
