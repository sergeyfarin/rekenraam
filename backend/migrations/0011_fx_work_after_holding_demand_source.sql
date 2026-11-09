-- +goose Up
-- #540: a held commodity's quote currency can arrive after its last posting.
-- Migration 0010 wakes coverage when the holding is posted; these triggers wake
-- it when the demanded set grows from either later source:
--   * a new instrument version that names the quote currency, and
--   * the first non-voided observation of the held commodity in a currency.
-- Both carry the holding's first posted date as a hint (reason
-- 'holding_entered', like the posting wakeup) and use the same coalescing.
-- Watchlist instruments and commodities held only in drafts, voided versions or
-- previews have no current posted posting and wake nothing. The version trigger
-- deliberately does not compare with the previous version: FXDemand selects the
-- version in effect on the owner's day, not the highest version_seq, so a
-- backdated version naming a currency must not hide the wakeup for a later
-- version that actually becomes current.
CREATE INDEX posting_versions_commodity_idx
  ON posting_versions (book_id, commodity_id);

-- +goose StatementBegin
CREATE TRIGGER fx_work_after_held_instrument_version_insert
AFTER INSERT ON investment_instrument_versions
WHEN NEW.quote_commodity_id IS NOT NULL
  AND EXISTS (
    SELECT 1 FROM commodities quote
    WHERE quote.id = NEW.quote_commodity_id AND quote.kind = 'currency'
  )
  AND NEW.quote_commodity_id != COALESCE(
    (SELECT b.default_currency_commodity_id
     FROM investment_instruments ii
     JOIN books b ON b.id = ii.book_id
     WHERE ii.id = NEW.instrument_id),
    0
  )
BEGIN
  INSERT INTO background_work_items (book_id, kind, payload_json, available_at, created_at, updated_at)
  SELECT ii.book_id, 'pricing.fx_coverage',
    json_object('reason', 'holding_entered', 'start_date', held.first_posted, 'currency_id', NEW.quote_commodity_id),
    strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
  FROM investment_instruments ii
  JOIN commodities held_kind ON held_kind.id = ii.commodity_id AND held_kind.kind <> 'currency'
  JOIN (
    SELECT pv.book_id, pv.commodity_id, MIN(je.entry_date) AS first_posted
    FROM current_transaction_versions tv
    JOIN transactions t ON t.id = tv.transaction_id AND t.deleted_at IS NULL
    JOIN journal_entries je ON je.transaction_version_id = tv.id
    JOIN posting_versions pv ON pv.journal_entry_id = je.id
    WHERE tv.status = 'posted'
      AND pv.book_id = (SELECT book_id FROM investment_instruments WHERE id = NEW.instrument_id)
      AND pv.commodity_id = (SELECT commodity_id FROM investment_instruments WHERE id = NEW.instrument_id)
    GROUP BY pv.book_id, pv.commodity_id
  ) held ON held.book_id = ii.book_id AND held.commodity_id = ii.commodity_id
  WHERE ii.id = NEW.instrument_id
    AND NOT EXISTS (
      SELECT 1 FROM background_work_items bw
      WHERE bw.book_id = held.book_id
        AND bw.kind = 'pricing.fx_coverage'
        AND bw.status = 'pending' AND bw.attempts = 0
        AND json_extract(bw.payload_json, '$.next') IS NULL
        AND json_extract(bw.payload_json, '$.retry') IS NULL
        AND json_extract(bw.payload_json, '$.currency_id') = NEW.quote_commodity_id
        AND json_extract(bw.payload_json, '$.start_date') <= held.first_posted
    )
  ON CONFLICT(book_id, kind, payload_json) WHERE status IN ('pending', 'running') DO NOTHING;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER fx_work_after_held_commodity_observation_insert
AFTER INSERT ON price_observations
WHEN NEW.voided_at IS NULL
  AND EXISTS (
    SELECT 1 FROM commodities base
    WHERE base.id = NEW.base_commodity_id AND base.kind <> 'currency'
  )
  AND EXISTS (
    SELECT 1 FROM commodities quote
    WHERE quote.id = NEW.quote_commodity_id AND quote.kind = 'currency'
  )
  AND NEW.quote_commodity_id != COALESCE(
    (SELECT b.default_currency_commodity_id FROM books b WHERE b.id = NEW.book_id),
    0
  )
  AND NOT EXISTS (
    SELECT 1 FROM price_observations prior
    WHERE prior.book_id = NEW.book_id
      AND prior.base_commodity_id = NEW.base_commodity_id
      AND prior.quote_commodity_id = NEW.quote_commodity_id
      AND prior.voided_at IS NULL
      AND prior.id <> NEW.id
  )
BEGIN
  INSERT INTO background_work_items (book_id, kind, payload_json, available_at, created_at, updated_at)
  SELECT NEW.book_id, 'pricing.fx_coverage',
    json_object('reason', 'holding_entered', 'start_date', held.first_posted, 'currency_id', NEW.quote_commodity_id),
    strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
  FROM (
    SELECT MIN(je.entry_date) AS first_posted
    FROM current_transaction_versions tv
    JOIN transactions t ON t.id = tv.transaction_id AND t.deleted_at IS NULL
    JOIN journal_entries je ON je.transaction_version_id = tv.id
    JOIN posting_versions pv ON pv.journal_entry_id = je.id
    WHERE pv.book_id = NEW.book_id AND pv.commodity_id = NEW.base_commodity_id
      AND tv.status = 'posted'
    GROUP BY pv.book_id
    HAVING MIN(je.entry_date) IS NOT NULL
  ) held
  WHERE NOT EXISTS (
    SELECT 1 FROM background_work_items bw
    WHERE bw.book_id = NEW.book_id
      AND bw.kind = 'pricing.fx_coverage'
      AND bw.status = 'pending' AND bw.attempts = 0
      AND json_extract(bw.payload_json, '$.next') IS NULL
      AND json_extract(bw.payload_json, '$.retry') IS NULL
      AND json_extract(bw.payload_json, '$.currency_id') = NEW.quote_commodity_id
      AND json_extract(bw.payload_json, '$.start_date') <= held.first_posted
  )
  ON CONFLICT(book_id, kind, payload_json) WHERE status IN ('pending', 'running') DO NOTHING;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER fx_work_after_held_commodity_observation_insert;
DROP TRIGGER fx_work_after_held_instrument_version_insert;
DROP INDEX posting_versions_commodity_idx;
