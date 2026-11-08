-- +goose Up
-- #532: a posted holding of a non-currency commodity (a security or crypto)
-- demands the rate of each currency it is valued through: its instrument's
-- quote currency and every currency a non-voided observation prices it in.
-- FXDemand owns that demand; this trigger is the posting-outbox wakeup for it,
-- so a trade that creates the demand enqueues FX coverage like a posted
-- currency does (fx_work_after_posting_version_insert). Every instrument
-- version is consulted because the owner-local reference day is not known
-- here; the sweep re-reads current demand, so an over-wide hint does nothing.
-- +goose StatementBegin
CREATE TRIGGER fx_work_after_holding_posting_version_insert
AFTER INSERT ON posting_versions
WHEN EXISTS (SELECT 1 FROM transaction_versions tv WHERE tv.id = NEW.transaction_version_id AND tv.status = 'posted')
  AND EXISTS (SELECT 1 FROM commodities c WHERE c.id = NEW.commodity_id AND c.kind <> 'currency')
BEGIN
  INSERT INTO background_work_items (book_id, kind, payload_json, available_at, created_at, updated_at)
  SELECT NEW.book_id, 'pricing.fx_coverage',
    json_object('reason', 'transaction_entered', 'start_date', je.entry_date, 'currency_id', quoted.commodity_id),
    strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
  FROM (
    SELECT iv.quote_commodity_id AS commodity_id
    FROM investment_instruments ii
    JOIN investment_instrument_versions iv ON iv.instrument_id = ii.id
    WHERE ii.book_id = NEW.book_id AND ii.commodity_id = NEW.commodity_id
      AND iv.quote_commodity_id IS NOT NULL
    UNION
    SELECT po.quote_commodity_id
    FROM price_observations po
    WHERE po.book_id = NEW.book_id AND po.base_commodity_id = NEW.commodity_id
      AND po.voided_at IS NULL
  ) quoted
  JOIN commodities c ON c.id = quoted.commodity_id AND c.kind = 'currency'
  JOIN journal_entries je ON je.id = NEW.journal_entry_id
  WHERE quoted.commodity_id != COALESCE(
      (SELECT default_currency_commodity_id FROM books WHERE id = NEW.book_id),
      0
    )
    AND NOT EXISTS (
      SELECT 1 FROM background_work_items bw
      WHERE bw.book_id = NEW.book_id AND bw.kind = 'pricing.fx_coverage'
        AND bw.status = 'pending' AND bw.attempts = 0
        AND json_extract(bw.payload_json, '$.next') IS NULL
        AND json_extract(bw.payload_json, '$.retry') IS NULL
        AND json_extract(bw.payload_json, '$.currency_id') = quoted.commodity_id
        AND json_extract(bw.payload_json, '$.start_date') <= je.entry_date
    )
  ON CONFLICT(book_id, kind, payload_json) WHERE status IN ('pending', 'running') DO NOTHING;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER fx_work_after_holding_posting_version_insert;
