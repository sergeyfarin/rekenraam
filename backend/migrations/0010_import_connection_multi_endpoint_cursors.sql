-- +goose Up

-- B-T212-INVST (Slice 4b): Trading 212 now fetches three independently
-- paginated endpoints (cash transactions, order fills, dividends), each with
-- its own incremental cursor. The original fetch_cursor column only fit one
-- endpoint. No production data exists yet for this table (pre-release schema
-- iteration — see docs/backlog.md T-21 and the memory this pattern is
-- recorded under), so this renames rather than bolting on a JSON blob.
ALTER TABLE import_connections RENAME COLUMN fetch_cursor TO transactions_cursor;
ALTER TABLE import_connections ADD COLUMN orders_cursor TEXT NOT NULL DEFAULT '';
ALTER TABLE import_connections ADD COLUMN dividends_cursor TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE import_connections DROP COLUMN dividends_cursor;
ALTER TABLE import_connections DROP COLUMN orders_cursor;
ALTER TABLE import_connections RENAME COLUMN transactions_cursor TO fetch_cursor;
