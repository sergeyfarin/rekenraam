-- V15: FX rate refresh settings, source assignments, and refresh state

-- Store FX refresh settings per book
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

-- Per-currency (or pair) source assignments with effective dates
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

-- Track refresh status per source/currency pair
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

-- Attach optional source_id to daily FX rates
ALTER TABLE fx_rates_daily ADD COLUMN source_id INTEGER;
CREATE INDEX IF NOT EXISTS idx_fx_daily_source ON fx_rates_daily(source_id);

-- Seed FX settings for the default book if missing
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
