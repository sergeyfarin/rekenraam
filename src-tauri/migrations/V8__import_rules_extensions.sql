-- V8: import rules extensions (priority, match types, amount/date/account)
PRAGMA foreign_keys = ON;

ALTER TABLE import_rules ADD COLUMN priority INTEGER NOT NULL DEFAULT 100;
ALTER TABLE import_rules ADD COLUMN match_type TEXT NOT NULL DEFAULT 'contains' CHECK (match_type IN ('contains','equals'));
ALTER TABLE import_rules ADD COLUMN amount_min_minor INTEGER;
ALTER TABLE import_rules ADD COLUMN amount_max_minor INTEGER;
ALTER TABLE import_rules ADD COLUMN date_from TEXT;
ALTER TABLE import_rules ADD COLUMN date_to TEXT;
ALTER TABLE import_rules ADD COLUMN match_account_id INTEGER REFERENCES accounts(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_import_rules_priority ON import_rules(priority, id);
