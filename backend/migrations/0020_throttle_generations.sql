-- +goose Up
-- #366: settlement must identify the window that admitted the attempt.
-- Preserve counts/deadlines on upgrade, including active blocks.
ALTER TABLE login_throttles ADD COLUMN generation TEXT NOT NULL DEFAULT '';
UPDATE login_throttles SET generation = lower(hex(randomblob(16)));

-- +goose Down
-- Development-only reversal; production rollback uses a verified backup.
ALTER TABLE login_throttles DROP COLUMN generation;
