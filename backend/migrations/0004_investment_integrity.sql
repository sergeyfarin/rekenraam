-- +goose Up

ALTER TABLE investment_lot_events ADD COLUMN cost_basis_method TEXT CHECK (
  cost_basis_method IS NULL OR cost_basis_method IN ('fifo', 'lifo', 'average_cost', 'specific_lot')
);

CREATE INDEX investment_lot_events_transaction_idx
  ON investment_lot_events (book_id, transaction_id)
  WHERE transaction_id IS NOT NULL;

CREATE TABLE investment_position_basis_state (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  cost_commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  method_family TEXT NOT NULL CHECK (method_family IN ('individual_lot', 'average_cost')),
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  UNIQUE (book_id, account_id, commodity_id, cost_commodity_id)
);

-- +goose Down

DROP TABLE IF EXISTS investment_position_basis_state;
DROP INDEX IF EXISTS investment_lot_events_transaction_idx;
ALTER TABLE investment_lot_events DROP COLUMN cost_basis_method;
