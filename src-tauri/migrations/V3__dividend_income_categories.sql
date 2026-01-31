-- V3: dividend income categories
PRAGMA foreign_keys = ON;

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
