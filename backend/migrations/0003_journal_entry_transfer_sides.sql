-- +goose Up
-- #481: a transfer's entries keep the owner's two choices that the journal's
-- days cannot show when both sides post on one day: which side converted an
-- exchange (its commodity_trading legs post on that side's day) and which
-- side paid a fee charged on top (the fee posts on that side's day). Every
-- entry of the version carries the same values; NULL is no choice recorded.
-- A reader still checks them against the journal's own postings.
ALTER TABLE journal_entries ADD COLUMN conversion_side TEXT
  CHECK (conversion_side IS NULL OR conversion_side IN ('sending', 'receiving'));
ALTER TABLE journal_entries ADD COLUMN fee_paid_by TEXT
  CHECK (fee_paid_by IS NULL OR fee_paid_by IN ('sending', 'receiving'));

-- +goose Down
ALTER TABLE journal_entries DROP COLUMN fee_paid_by;
ALTER TABLE journal_entries DROP COLUMN conversion_side;
