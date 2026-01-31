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