-- +goose Up

CREATE TABLE cost_basis_profile_versions (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  profile_id INTEGER NOT NULL REFERENCES cost_basis_profiles(id) ON DELETE RESTRICT,
  version_seq INTEGER NOT NULL CHECK (version_seq > 0),
  name TEXT NOT NULL CHECK (length(trim(name)) > 0 AND name = trim(name)),
  method TEXT NOT NULL CHECK (method IN ('fifo', 'lifo', 'average_cost', 'specific_lot')),
  is_default INTEGER NOT NULL CHECK (is_default IN (0, 1)),
  status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
  description TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  recorded_at TEXT NOT NULL,
  changed_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  change_reason TEXT NOT NULL,
  audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  UNIQUE (profile_id, version_seq)
);

INSERT INTO cost_basis_profile_versions (
  book_id, profile_id, version_seq, name, method, is_default, status,
  description, metadata_json, recorded_at, changed_by_user_id,
  change_reason, audit_event_id
)
SELECT book_id, id, 1, name, method, is_default, status, description,
  metadata_json, updated_at, updated_by_user_id, 'initial version',
  updated_audit_event_id
FROM cost_basis_profiles;

ALTER TABLE cost_basis_profiles ADD COLUMN current_version_id INTEGER
  REFERENCES cost_basis_profile_versions(id) ON DELETE RESTRICT;

UPDATE cost_basis_profiles
SET current_version_id = (
  SELECT version.id
  FROM cost_basis_profile_versions version
  WHERE version.profile_id = cost_basis_profiles.id
  ORDER BY version.version_seq DESC
  LIMIT 1
);

CREATE UNIQUE INDEX cost_basis_profiles_current_version_idx
  ON cost_basis_profiles(current_version_id)
  WHERE current_version_id IS NOT NULL;

CREATE TABLE investment_disposal_decisions (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE RESTRICT,
  transaction_version_id INTEGER NOT NULL REFERENCES transaction_versions(id) ON DELETE RESTRICT,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  cost_commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  event_date TEXT NOT NULL CHECK (event_date GLOB '????-??-??'),
  quantity_value TEXT NOT NULL CHECK (length(quantity_value) BETWEEN 1 AND 38),
  quantity_scale INTEGER NOT NULL CHECK (quantity_scale BETWEEN 0 AND 24),
  disposed_basis_value TEXT NOT NULL CHECK (length(disposed_basis_value) BETWEEN 1 AND 38),
  disposed_basis_scale INTEGER NOT NULL CHECK (disposed_basis_scale BETWEEN 0 AND 12),
  cost_basis_method TEXT NOT NULL CHECK (cost_basis_method IN ('fifo', 'lifo', 'average_cost', 'specific_lot')),
  resolution_tier TEXT NOT NULL CHECK (resolution_tier IN ('transaction', 'account', 'global', 'fallback')),
  account_version_id INTEGER REFERENCES account_versions(id) ON DELETE RESTRICT,
  profile_id INTEGER REFERENCES cost_basis_profiles(id) ON DELETE RESTRICT,
  profile_version_id INTEGER REFERENCES cost_basis_profile_versions(id) ON DELETE RESTRICT,
  source_effective_from TEXT CHECK (source_effective_from IS NULL OR source_effective_from GLOB '????-??-??'),
  source_recorded_at TEXT,
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER NOT NULL REFERENCES audit_events(id) ON DELETE RESTRICT,
  UNIQUE (transaction_id),
  UNIQUE (transaction_version_id),
  CHECK (
    (resolution_tier = 'account' AND account_version_id IS NOT NULL AND profile_id IS NULL AND profile_version_id IS NULL)
    OR (resolution_tier = 'global' AND account_version_id IS NULL AND profile_id IS NOT NULL AND profile_version_id IS NOT NULL)
    OR (resolution_tier IN ('transaction', 'fallback') AND account_version_id IS NULL AND profile_id IS NULL AND profile_version_id IS NULL)
  )
);

CREATE TABLE investment_disposal_allocations (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  decision_id INTEGER NOT NULL REFERENCES investment_disposal_decisions(id) ON DELETE RESTRICT,
  lot_event_id INTEGER NOT NULL REFERENCES investment_lot_events(id) ON DELETE RESTRICT,
  lot_id INTEGER NOT NULL REFERENCES investment_lots(id) ON DELETE RESTRICT,
  allocation_seq INTEGER NOT NULL CHECK (allocation_seq > 0),
  quantity_value TEXT NOT NULL CHECK (length(quantity_value) BETWEEN 1 AND 38),
  quantity_scale INTEGER NOT NULL CHECK (quantity_scale BETWEEN 0 AND 24),
  cost_basis_value INTEGER NOT NULL,
  cost_basis_scale INTEGER NOT NULL CHECK (cost_basis_scale BETWEEN 0 AND 12),
  UNIQUE (decision_id, allocation_seq),
  UNIQUE (lot_event_id)
);

CREATE INDEX investment_disposal_decisions_event_idx
  ON investment_disposal_decisions(book_id, event_date, id);

-- +goose Down

DROP INDEX IF EXISTS investment_disposal_decisions_event_idx;
DROP TABLE IF EXISTS investment_disposal_allocations;
DROP TABLE IF EXISTS investment_disposal_decisions;
DROP INDEX IF EXISTS cost_basis_profiles_current_version_idx;
ALTER TABLE cost_basis_profiles DROP COLUMN current_version_id;
DROP TABLE IF EXISTS cost_basis_profile_versions;
