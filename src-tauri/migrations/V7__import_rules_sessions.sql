-- V7: import rules + import sessions
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS import_rules (
  id                 INTEGER PRIMARY KEY,
  book_id            INTEGER NOT NULL,
  rule_kind          TEXT NOT NULL CHECK (rule_kind IN ('payee','memo')),
  match_text         TEXT NOT NULL,
  target_account_id  INTEGER,
  target_category_id INTEGER,
  target_payee_id    INTEGER,
  created_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(target_account_id) REFERENCES accounts(id) ON DELETE SET NULL,
  FOREIGN KEY(target_category_id) REFERENCES categories(id) ON DELETE SET NULL,
  FOREIGN KEY(target_payee_id) REFERENCES payees(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_import_rules_book_kind ON import_rules(book_id, rule_kind);

CREATE TABLE IF NOT EXISTS import_sessions (
  id         INTEGER PRIMARY KEY,
  book_id    INTEGER NOT NULL,
  source     TEXT,
  status     TEXT NOT NULL DEFAULT 'started' CHECK (status IN ('started','committed','abandoned')),
  started_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  committed_at TEXT,
  notes      TEXT,
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE
);

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
