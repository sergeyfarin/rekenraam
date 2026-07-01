-- +goose Up

-- B-T212-SCHED (Slice 4a, docs/trading212-import-plan.md): per-connection
-- scheduled auto-refresh toggle. Cadence is computed in application code from
-- last_fetched_at (24h-since-last-attempt), not a stored schedule column —
-- see ImportService.runDueTrading212AutoRefreshes.
ALTER TABLE import_connections ADD COLUMN auto_refresh_enabled INTEGER NOT NULL DEFAULT 0 CHECK (auto_refresh_enabled IN (0, 1));

-- +goose Down
-- SQLite does not support DROP COLUMN; auto_refresh_enabled is left in place
-- on the down path (harmless: defaults to 0, matches pre-migration behavior).
