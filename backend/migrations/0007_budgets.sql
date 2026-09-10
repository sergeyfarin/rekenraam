-- +goose Up
CREATE TABLE budget_targets (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  category_account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  period_start TEXT NOT NULL CHECK (period_start GLOB '????-??-01'),
  quantity_value TEXT NOT NULL CHECK (length(quantity_value) BETWEEN 1 AND 39),
  quantity_scale INTEGER NOT NULL CHECK (quantity_scale BETWEEN 0 AND 12),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER NOT NULL REFERENCES audit_events(id) ON DELETE RESTRICT,
  UNIQUE (book_id, category_account_id, commodity_id, period_start)
);

CREATE INDEX budget_targets_book_period_idx
  ON budget_targets (book_id, period_start, category_account_id, commodity_id);

CREATE TABLE account_budget_treatment_versions (
  id INTEGER PRIMARY KEY,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  version_seq INTEGER NOT NULL CHECK (version_seq > 0),
  effective_from TEXT NOT NULL CHECK (effective_from GLOB '????-??-??'),
  treatment TEXT NOT NULL CHECK (treatment IN ('on_budget', 'off_budget', 'excluded')),
  recorded_at TEXT NOT NULL,
  changed_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  change_reason TEXT NOT NULL,
  change_audit_event_id INTEGER NOT NULL REFERENCES audit_events(id) ON DELETE RESTRICT,
  UNIQUE (account_id, version_seq)
);

CREATE INDEX account_budget_treatment_asof_idx
  ON account_budget_treatment_versions (account_id, effective_from DESC, version_seq DESC);

-- +goose Down
DROP TABLE account_budget_treatment_versions;
DROP TABLE budget_targets;
