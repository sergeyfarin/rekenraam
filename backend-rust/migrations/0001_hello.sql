CREATE TABLE IF NOT EXISTS app_info (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

INSERT OR IGNORE INTO app_info (key, value)
VALUES ('hello', 'hello from sqlite');
