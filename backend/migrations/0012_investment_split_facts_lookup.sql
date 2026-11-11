-- +goose Up
-- #561: split reads (InstrumentSplits, TradeQuotePendingSplits) select a
-- book's split facts by instrument and effective date. The table had only its
-- primary key, so each read scanned every book's split facts.
CREATE INDEX investment_split_facts_lookup_idx
  ON investment_split_facts (book_id, commodity_id, effective_on);

-- +goose Down
DROP INDEX investment_split_facts_lookup_idx;
