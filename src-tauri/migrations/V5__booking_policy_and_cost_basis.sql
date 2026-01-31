-- V5: booking policy + lot cost basis
PRAGMA foreign_keys = ON;

ALTER TABLE accounts ADD COLUMN booking_policy TEXT NOT NULL DEFAULT 'fifo' CHECK (booking_policy IN ('fifo','lifo','strict','average'));

ALTER TABLE lots ADD COLUMN cost_basis_minor INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_accounts_booking_policy ON accounts(booking_policy);
