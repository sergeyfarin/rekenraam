-- V6: balance constraints
PRAGMA foreign_keys = ON;

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
