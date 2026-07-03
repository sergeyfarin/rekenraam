-- +goose Up

-- B-T212-INVST (Slice 4b, docs/import-connection-accounts-plan.md): a
-- connection needs a settlement cash account (one per connection, set at
-- connect time) and a growing set of holding accounts (one per instrument
-- seen, resolved lazily as new tickers appear in fetched history).
ALTER TABLE import_connections ADD COLUMN cash_account_id INTEGER
  REFERENCES accounts(id) ON DELETE SET NULL;

CREATE TABLE import_connection_holdings (
  id INTEGER PRIMARY KEY,
  connection_id INTEGER NOT NULL REFERENCES import_connections(id) ON DELETE CASCADE,
  commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  holding_account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  created_at TEXT NOT NULL,
  UNIQUE (connection_id, commodity_id)
);

CREATE INDEX import_connection_holdings_connection_idx
  ON import_connection_holdings (connection_id);

-- +goose Down
DROP INDEX IF EXISTS import_connection_holdings_connection_idx;
DROP TABLE IF EXISTS import_connection_holdings;
-- SQLite does not support DROP COLUMN; cash_account_id is left in place on
-- the down path (harmless: defaults to NULL, matches pre-migration behavior).
