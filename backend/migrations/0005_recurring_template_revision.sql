-- +goose Up
-- A PATCH merges a validated snapshot. Refuse a stale merge rather than
-- overwriting a concurrent template edit or generation watermark advance.
ALTER TABLE recurring_templates ADD COLUMN revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0);

-- +goose Down
ALTER TABLE recurring_templates DROP COLUMN revision;
