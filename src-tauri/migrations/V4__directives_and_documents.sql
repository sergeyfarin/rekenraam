-- V4: directives + documents/events/notes/pad/balance checks
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS account_directives (
  id             INTEGER PRIMARY KEY,
  book_id        INTEGER NOT NULL,
  account_id     INTEGER NOT NULL,
  directive_type TEXT NOT NULL CHECK (directive_type IN ('open','close')),
  directive_date TEXT NOT NULL,
  note           TEXT,
  metadata       TEXT,
  created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_account_directives_account_date
  ON account_directives(account_id, directive_date);

CREATE TABLE IF NOT EXISTS balance_checks (
  id            INTEGER PRIMARY KEY,
  book_id       INTEGER NOT NULL,
  account_id    INTEGER NOT NULL,
  as_of_date    TEXT NOT NULL,
  balance_minor INTEGER NOT NULL,
  memo          TEXT,
  status        TEXT NOT NULL DEFAULT 'recorded' CHECK (status IN ('recorded','matched','failed')),
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_balance_checks_account_date
  ON balance_checks(account_id, as_of_date);

CREATE TABLE IF NOT EXISTS pad_directives (
  id                    INTEGER PRIMARY KEY,
  book_id               INTEGER NOT NULL,
  account_id            INTEGER NOT NULL,
  pad_account_id        INTEGER NOT NULL,
  as_of_date            TEXT NOT NULL,
  target_balance_minor  INTEGER NOT NULL,
  tx_id                 INTEGER,
  memo                  TEXT,
  created_at            TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE,
  FOREIGN KEY(pad_account_id) REFERENCES accounts(id) ON DELETE CASCADE,
  FOREIGN KEY(tx_id) REFERENCES transactions(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_pad_directives_account_date
  ON pad_directives(account_id, as_of_date);

CREATE TABLE IF NOT EXISTS notes (
  id          INTEGER PRIMARY KEY,
  book_id     INTEGER NOT NULL,
  account_id  INTEGER,
  tx_id       INTEGER,
  note        TEXT NOT NULL,
  note_date   TEXT,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE SET NULL,
  FOREIGN KEY(tx_id) REFERENCES transactions(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_notes_account ON notes(account_id);
CREATE INDEX IF NOT EXISTS idx_notes_tx ON notes(tx_id);

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

-- Book state bumpers for new tables
CREATE TRIGGER IF NOT EXISTS trg_bump_account_directives_ins
AFTER INSERT ON account_directives
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_account_directives_upd
AFTER UPDATE ON account_directives
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_account_directives_del
AFTER DELETE ON account_directives
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = OLD.book_id;
END;

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

CREATE TRIGGER IF NOT EXISTS trg_bump_pad_directives_ins
AFTER INSERT ON pad_directives
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_pad_directives_upd
AFTER UPDATE ON pad_directives
BEGIN
  UPDATE book_state SET change_seq = change_seq + 1, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ','now')) WHERE book_id = NEW.book_id;
END;
CREATE TRIGGER IF NOT EXISTS trg_bump_pad_directives_del
AFTER DELETE ON pad_directives
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
