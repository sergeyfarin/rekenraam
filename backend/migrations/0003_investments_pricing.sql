-- +goose Up
CREATE TABLE IF NOT EXISTS market_data_sources (
  id INTEGER PRIMARY KEY,
  code TEXT NOT NULL UNIQUE CHECK (
    length(code) BETWEEN 1 AND 64
    AND code = trim(code)
    AND code GLOB '[a-z]*'
    AND code NOT GLOB '*[^a-z0-9_-]*'
  ),
  name TEXT NOT NULL CHECK (length(trim(name)) > 0 AND name = trim(name)),
  kind TEXT NOT NULL CHECK (kind IN ('manual', 'provider', 'import')),
  provider_key TEXT,
  base_url TEXT,
  status TEXT NOT NULL CHECK (status IN ('active', 'disabled')),
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

INSERT OR IGNORE INTO market_data_sources (
  id, code, name, kind, status, metadata_json, created_at
) VALUES (
  1, 'manual', 'Manual', 'manual', 'active', '{}', '0001-01-01T00:00:00Z'
);

CREATE TABLE IF NOT EXISTS investment_instruments (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_request_id TEXT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  UNIQUE (book_id, commodity_id)
);

CREATE INDEX IF NOT EXISTS investment_instruments_book_idx
  ON investment_instruments (book_id);

CREATE TABLE IF NOT EXISTS investment_instrument_versions (
  id INTEGER PRIMARY KEY,
  instrument_id INTEGER NOT NULL REFERENCES investment_instruments(id) ON DELETE RESTRICT,
  version_seq INTEGER NOT NULL CHECK (version_seq > 0),
  effective_from TEXT NOT NULL CHECK (effective_from GLOB '????-??-??'),
  recorded_at TEXT NOT NULL,
  changed_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  change_reason TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
  instrument_type TEXT NOT NULL CHECK (instrument_type IN ('stock', 'etf', 'fund', 'bond', 'option', 'future', 'other')),
  display_name TEXT NOT NULL CHECK (length(trim(display_name)) > 0 AND display_name = trim(display_name)),
  symbol TEXT CHECK (symbol IS NULL OR (length(trim(symbol)) > 0 AND symbol = trim(symbol))),
  exchange_code TEXT,
  mic TEXT,
  issuer TEXT,
  country_code TEXT,
  quote_commodity_id INTEGER REFERENCES commodities(id) ON DELETE RESTRICT,
  trading_commodity_id INTEGER REFERENCES commodities(id) ON DELETE RESTRICT,
  quantity_scale INTEGER NOT NULL CHECK (quantity_scale BETWEEN 0 AND 12),
  price_scale INTEGER NOT NULL CHECK (price_scale BETWEEN 0 AND 12),
  identifiers_json TEXT NOT NULL DEFAULT '{}',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  change_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  UNIQUE (instrument_id, version_seq)
);

CREATE INDEX IF NOT EXISTS investment_instrument_versions_instrument_seq_idx
  ON investment_instrument_versions (instrument_id, version_seq DESC);

CREATE INDEX IF NOT EXISTS investment_instrument_versions_symbol_idx
  ON investment_instrument_versions (symbol COLLATE NOCASE, exchange_code, mic);

CREATE VIEW IF NOT EXISTS current_investment_instrument_versions AS
SELECT iiv.*
FROM investment_instrument_versions iiv
WHERE iiv.id = (
  SELECT current_iiv.id
  FROM investment_instrument_versions current_iiv
  WHERE current_iiv.instrument_id = iiv.instrument_id
    AND current_iiv.effective_from <= date('now')
  ORDER BY current_iiv.effective_from DESC, current_iiv.version_seq DESC
  LIMIT 1
);

CREATE TABLE IF NOT EXISTS instrument_source_links (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  instrument_id INTEGER NOT NULL REFERENCES investment_instruments(id) ON DELETE RESTRICT,
  source_id INTEGER NOT NULL REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  provider_instrument_id TEXT,
  symbol TEXT NOT NULL CHECK (length(trim(symbol)) > 0 AND symbol = trim(symbol)),
  exchange_code TEXT,
  mic TEXT,
  confidence_bps INTEGER NOT NULL DEFAULT 10000 CHECK (confidence_bps BETWEEN 0 AND 10000),
  status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
  effective_from TEXT NOT NULL CHECK (effective_from GLOB '????-??-??'),
  effective_to TEXT CHECK (effective_to IS NULL OR effective_to GLOB '????-??-??'),
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  CHECK (effective_to IS NULL OR effective_to >= effective_from)
);

CREATE INDEX IF NOT EXISTS instrument_source_links_lookup_idx
  ON instrument_source_links (book_id, source_id, symbol COLLATE NOCASE, exchange_code, mic, status);

CREATE UNIQUE INDEX IF NOT EXISTS instrument_source_links_active_provider_idx
  ON instrument_source_links (book_id, source_id, provider_instrument_id)
  WHERE status = 'active' AND provider_instrument_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS instrument_search_cache (
  id INTEGER PRIMARY KEY,
  source_id INTEGER NOT NULL REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  query TEXT NOT NULL,
  provider_instrument_id TEXT,
  symbol TEXT NOT NULL,
  exchange_code TEXT,
  mic TEXT,
  display_name TEXT NOT NULL,
  instrument_type TEXT NOT NULL,
  quote_code TEXT,
  country_code TEXT,
  confidence_bps INTEGER NOT NULL DEFAULT 0 CHECK (confidence_bps BETWEEN 0 AND 10000),
  raw_json TEXT NOT NULL DEFAULT '{}',
  fetched_at TEXT NOT NULL,
  expires_at TEXT
);

CREATE INDEX IF NOT EXISTS instrument_search_cache_query_idx
  ON instrument_search_cache (source_id, query COLLATE NOCASE, fetched_at DESC);

CREATE TABLE IF NOT EXISTS pricing_policies (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT UNIQUE,
  base_commodity_id INTEGER REFERENCES commodities(id) ON DELETE RESTRICT,
  default_source_id INTEGER REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  refresh_enabled INTEGER NOT NULL DEFAULT 0 CHECK (refresh_enabled IN (0, 1)),
  refresh_hour_utc INTEGER NOT NULL DEFAULT 4 CHECK (refresh_hour_utc BETWEEN 0 AND 23),
  refresh_minute_utc INTEGER NOT NULL DEFAULT 0 CHECK (refresh_minute_utc BETWEEN 0 AND 59),
  max_backfill_days INTEGER NOT NULL DEFAULT 370 CHECK (max_backfill_days >= 1),
  staleness_max_days INTEGER NOT NULL DEFAULT 3 CHECK (staleness_max_days >= 1),
  triangulation_max_hops INTEGER NOT NULL DEFAULT 1 CHECK (triangulation_max_hops BETWEEN 0 AND 4),
  rounding_mode TEXT NOT NULL DEFAULT 'half_up' CHECK (rounding_mode IN ('half_up', 'half_even', 'down', 'up')),
  prefer_official_fx INTEGER NOT NULL DEFAULT 1 CHECK (prefer_official_fx IN (0, 1)),
  weekend_policy TEXT NOT NULL DEFAULT 'skip' CHECK (weekend_policy IN ('skip', 'fill_previous', 'download')),
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS pricing_source_assignments (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  base_commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  quote_commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  source_id INTEGER NOT NULL REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  priority INTEGER NOT NULL DEFAULT 100 CHECK (priority >= 0),
  status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
  effective_from TEXT NOT NULL CHECK (effective_from GLOB '????-??-??'),
  effective_to TEXT CHECK (effective_to IS NULL OR effective_to GLOB '????-??-??'),
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  CHECK (effective_to IS NULL OR effective_to >= effective_from)
);

CREATE INDEX IF NOT EXISTS pricing_source_assignments_lookup_idx
  ON pricing_source_assignments (book_id, base_commodity_id, quote_commodity_id, status, effective_from DESC, priority);

CREATE TABLE IF NOT EXISTS price_series (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  base_commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  quote_commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  source_id INTEGER REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  quote_type TEXT NOT NULL CHECK (quote_type IN ('close', 'adjusted_close', 'nav', 'official_fixing', 'spot_mid', 'bid', 'ask', 'manual', 'trade_implied', 'valuation_override')),
  adjustment_basis TEXT NOT NULL DEFAULT 'raw' CHECK (adjustment_basis IN ('raw', 'split_adjusted', 'dividend_adjusted', 'total_return', 'not_applicable')),
  market_code TEXT,
  provider_series_id TEXT,
  status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS price_series_lookup_idx
  ON price_series (book_id, base_commodity_id, quote_commodity_id, quote_type, adjustment_basis, status, source_id);

CREATE UNIQUE INDEX IF NOT EXISTS price_series_provider_idx
  ON price_series (book_id, source_id, provider_series_id)
  WHERE source_id IS NOT NULL AND provider_series_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS price_observations (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  series_id INTEGER NOT NULL REFERENCES price_series(id) ON DELETE RESTRICT,
  base_commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  quote_commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  quote_type TEXT NOT NULL CHECK (quote_type IN ('close', 'adjusted_close', 'nav', 'official_fixing', 'spot_mid', 'bid', 'ask', 'manual', 'trade_implied', 'valuation_override')),
  adjustment_basis TEXT NOT NULL DEFAULT 'raw' CHECK (adjustment_basis IN ('raw', 'split_adjusted', 'dividend_adjusted', 'total_return', 'not_applicable')),
  price_value INTEGER NOT NULL,
  price_scale INTEGER NOT NULL CHECK (price_scale BETWEEN 0 AND 18),
  base_quantity_value INTEGER NOT NULL DEFAULT 1 CHECK (base_quantity_value > 0),
  base_quantity_scale INTEGER NOT NULL DEFAULT 0 CHECK (base_quantity_scale BETWEEN 0 AND 18),
  valuation_date TEXT NOT NULL CHECK (valuation_date GLOB '????-??-??'),
  observed_at TEXT,
  source_published_at TEXT,
  source_id INTEGER REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  provider_observation_id TEXT,
  is_manual INTEGER NOT NULL DEFAULT 0 CHECK (is_manual IN (0, 1)),
  is_derived INTEGER NOT NULL DEFAULT 0 CHECK (is_derived IN (0, 1)),
  supersedes_observation_id INTEGER REFERENCES price_observations(id) ON DELETE RESTRICT,
  ingest_run_id INTEGER,
  derivation_json TEXT NOT NULL DEFAULT '{}',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  voided_at TEXT,
  voided_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  void_reason TEXT NOT NULL DEFAULT '',
  recorded_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS price_observations_lookup_idx
  ON price_observations (book_id, base_commodity_id, quote_commodity_id, quote_type, adjustment_basis, valuation_date DESC, recorded_at DESC, id DESC);

CREATE UNIQUE INDEX IF NOT EXISTS price_observations_provider_event_idx
  ON price_observations (source_id, provider_observation_id)
  WHERE provider_observation_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS market_data_ingest_runs (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  source_id INTEGER NOT NULL REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  trigger TEXT NOT NULL CHECK (trigger IN ('manual', 'scheduled', 'provider_callback', 'import')),
  status TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed', 'partial')),
  started_at TEXT NOT NULL,
  finished_at TEXT,
  items_total INTEGER NOT NULL DEFAULT 0 CHECK (items_total >= 0),
  items_succeeded INTEGER NOT NULL DEFAULT 0 CHECK (items_succeeded >= 0),
  items_failed INTEGER NOT NULL DEFAULT 0 CHECK (items_failed >= 0),
  last_error TEXT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS market_data_ingest_runs_book_idx
  ON market_data_ingest_runs (book_id, started_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS market_data_ingest_items (
  id INTEGER PRIMARY KEY,
  run_id INTEGER NOT NULL REFERENCES market_data_ingest_runs(id) ON DELETE RESTRICT,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  item_kind TEXT NOT NULL CHECK (item_kind IN ('instrument', 'price', 'dividend', 'corporate_action')),
  status TEXT NOT NULL CHECK (status IN ('succeeded', 'failed', 'skipped')),
  provider_item_id TEXT,
  local_ref_table TEXT,
  local_ref_id INTEGER,
  error TEXT,
  raw_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS market_data_ingest_items_run_idx
  ON market_data_ingest_items (run_id, item_kind, status);

CREATE TABLE IF NOT EXISTS pricing_refresh_state (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  base_commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  quote_commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  source_id INTEGER NOT NULL REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  last_success_date TEXT CHECK (last_success_date IS NULL OR last_success_date GLOB '????-??-??'),
  last_attempt_at TEXT,
  last_error TEXT,
  updated_at TEXT NOT NULL,
  UNIQUE (book_id, base_commodity_id, quote_commodity_id, source_id)
);

CREATE TABLE IF NOT EXISTS cost_basis_profiles (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  name TEXT NOT NULL CHECK (length(trim(name)) > 0 AND name = trim(name)),
  method TEXT NOT NULL CHECK (method IN ('fifo', 'lifo', 'average_cost', 'specific_lot')),
  is_default INTEGER NOT NULL CHECK (is_default IN (0, 1)),
  status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
  description TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  UNIQUE (book_id, name COLLATE NOCASE)
);

CREATE UNIQUE INDEX IF NOT EXISTS cost_basis_profiles_default_idx
  ON cost_basis_profiles (book_id)
  WHERE is_default = 1 AND status = 'active';

CREATE TABLE IF NOT EXISTS dividend_defaults (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  commodity_id INTEGER REFERENCES commodities(id) ON DELETE RESTRICT,
  income_account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  withholding_account_id INTEGER REFERENCES accounts(id) ON DELETE RESTRICT,
  default_withholding_value INTEGER,
  default_withholding_scale INTEGER CHECK (default_withholding_scale IS NULL OR default_withholding_scale BETWEEN 0 AND 12),
  withholding_rate_bps INTEGER CHECK (withholding_rate_bps IS NULL OR withholding_rate_bps BETWEEN 0 AND 10000),
  tax_country_code TEXT,
  tax_treatment TEXT,
  status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
  effective_from TEXT NOT NULL CHECK (effective_from GLOB '????-??-??'),
  effective_to TEXT CHECK (effective_to IS NULL OR effective_to GLOB '????-??-??'),
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  CHECK (effective_to IS NULL OR effective_to >= effective_from)
);

CREATE INDEX IF NOT EXISTS dividend_defaults_lookup_idx
  ON dividend_defaults (book_id, commodity_id, status, effective_from DESC);

CREATE TABLE IF NOT EXISTS investment_lots (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  opened_on TEXT NOT NULL CHECK (opened_on GLOB '????-??-??'),
  source_transaction_id INTEGER REFERENCES transactions(id) ON DELETE RESTRICT,
  status TEXT NOT NULL CHECK (status IN ('open', 'closed')),
  quantity_value INTEGER NOT NULL,
  quantity_scale INTEGER NOT NULL CHECK (quantity_scale BETWEEN 0 AND 12),
  remaining_quantity_value INTEGER NOT NULL,
  remaining_quantity_scale INTEGER NOT NULL CHECK (remaining_quantity_scale BETWEEN 0 AND 12),
  cost_basis_value INTEGER NOT NULL,
  cost_basis_scale INTEGER NOT NULL CHECK (cost_basis_scale BETWEEN 0 AND 12),
  remaining_cost_basis_value INTEGER NOT NULL,
  remaining_cost_basis_scale INTEGER NOT NULL CHECK (remaining_cost_basis_scale BETWEEN 0 AND 12),
  cost_commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS investment_lots_position_idx
  ON investment_lots (book_id, account_id, commodity_id, status, opened_on, id);

CREATE TABLE IF NOT EXISTS investment_lot_events (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  lot_id INTEGER NOT NULL REFERENCES investment_lots(id) ON DELETE RESTRICT,
  event_kind TEXT NOT NULL CHECK (event_kind IN ('acquisition', 'disposal', 'split_adjustment', 'reinvested_dividend', 'manual_adjustment')),
  transaction_id INTEGER REFERENCES transactions(id) ON DELETE RESTRICT,
  event_date TEXT NOT NULL CHECK (event_date GLOB '????-??-??'),
  quantity_value INTEGER NOT NULL,
  quantity_scale INTEGER NOT NULL CHECK (quantity_scale BETWEEN 0 AND 12),
  cost_basis_value INTEGER NOT NULL DEFAULT 0,
  cost_basis_scale INTEGER NOT NULL DEFAULT 0 CHECK (cost_basis_scale BETWEEN 0 AND 12),
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS investment_lot_events_lot_idx
  ON investment_lot_events (lot_id, event_date, id);

CREATE TABLE IF NOT EXISTS investment_provider_events (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  source_id INTEGER NOT NULL REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  provider_event_id TEXT,
  instrument_id INTEGER REFERENCES investment_instruments(id) ON DELETE RESTRICT,
  event_family TEXT NOT NULL CHECK (event_family IN ('dividend', 'distribution', 'split', 'merger', 'spin_off', 'ticker_change', 'delisting', 'cash_in_lieu', 'return_of_capital', 'corporate_action')),
  event_date TEXT NOT NULL CHECK (event_date GLOB '????-??-??'),
  status TEXT NOT NULL CHECK (status IN ('new', 'matched', 'ignored', 'superseded')),
  normalized_json TEXT NOT NULL DEFAULT '{}',
  raw_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS investment_provider_events_source_event_idx
  ON investment_provider_events (source_id, provider_event_id)
  WHERE provider_event_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS investment_provider_events_book_idx
  ON investment_provider_events (book_id, event_family, event_date DESC, id DESC);

CREATE TABLE IF NOT EXISTS investment_event_suggestions (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  provider_event_id INTEGER NOT NULL REFERENCES investment_provider_events(id) ON DELETE RESTRICT,
  instrument_id INTEGER REFERENCES investment_instruments(id) ON DELETE RESTRICT,
  confidence_bps INTEGER NOT NULL CHECK (confidence_bps BETWEEN 0 AND 10000),
  status TEXT NOT NULL CHECK (status IN ('suggested', 'accepted', 'ignored', 'auto_posted', 'failed', 'superseded')),
  proposed_transaction_json TEXT NOT NULL DEFAULT '{}',
  generated_transaction_id INTEGER REFERENCES transactions(id) ON DELETE RESTRICT,
  failure_reason TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS investment_event_suggestions_book_idx
  ON investment_event_suggestions (book_id, status, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS investment_automation_rules (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  source_id INTEGER REFERENCES market_data_sources(id) ON DELETE RESTRICT,
  instrument_id INTEGER REFERENCES investment_instruments(id) ON DELETE RESTRICT,
  event_family TEXT NOT NULL CHECK (event_family IN ('dividend', 'distribution', 'split', 'merger', 'spin_off', 'ticker_change', 'delisting', 'cash_in_lieu', 'return_of_capital', 'corporate_action')),
  mode TEXT NOT NULL CHECK (mode IN ('suggest', 'auto_post')),
  confidence_threshold_bps INTEGER NOT NULL CHECK (confidence_threshold_bps BETWEEN 0 AND 10000),
  required_accounts_json TEXT NOT NULL DEFAULT '{}',
  status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
  effective_from TEXT NOT NULL CHECK (effective_from GLOB '????-??-??'),
  effective_to TEXT CHECK (effective_to IS NULL OR effective_to GLOB '????-??-??'),
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  CHECK (effective_to IS NULL OR effective_to >= effective_from)
);

CREATE INDEX IF NOT EXISTS investment_automation_rules_lookup_idx
  ON investment_automation_rules (book_id, event_family, status, source_id, instrument_id);

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS investment_instrument_versions_no_update
BEFORE UPDATE ON investment_instrument_versions
BEGIN
  SELECT RAISE(ABORT, 'investment_instrument_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS investment_instrument_versions_no_delete
BEFORE DELETE ON investment_instrument_versions
BEGIN
  SELECT RAISE(ABORT, 'investment_instrument_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS investment_instruments_commodity_must_be_security
BEFORE INSERT ON investment_instruments
WHEN NOT EXISTS (
  SELECT 1
  FROM commodities c
  WHERE c.id = NEW.commodity_id
    AND c.book_id = NEW.book_id
    AND c.kind = 'security'
)
BEGIN
  SELECT RAISE(ABORT, 'investment instrument commodity must be a security in the same book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS investment_lots_same_book
BEFORE INSERT ON investment_lots
WHEN NOT EXISTS (
  SELECT 1
  FROM accounts a
  JOIN commodities security ON security.id = NEW.commodity_id
  JOIN commodities cost ON cost.id = NEW.cost_commodity_id
  WHERE a.id = NEW.account_id
    AND a.book_id = NEW.book_id
    AND security.book_id = NEW.book_id
    AND security.kind = 'security'
    AND cost.book_id = NEW.book_id
)
BEGIN
  SELECT RAISE(ABORT, 'investment lot references must belong to one book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS investment_lot_events_same_book
BEFORE INSERT ON investment_lot_events
WHEN NOT EXISTS (
  SELECT 1
  FROM investment_lots lot
  WHERE lot.id = NEW.lot_id
    AND lot.book_id = NEW.book_id
)
BEGIN
  SELECT RAISE(ABORT, 'investment lot event must reference a lot in the same book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS pricing_source_assignments_same_book
BEFORE INSERT ON pricing_source_assignments
WHEN NOT EXISTS (
  SELECT 1
  FROM commodities b
  JOIN commodities q ON q.id = NEW.quote_commodity_id
  WHERE b.id = NEW.base_commodity_id
    AND b.book_id = NEW.book_id
    AND q.book_id = NEW.book_id
)
BEGIN
  SELECT RAISE(ABORT, 'pricing source assignment commodities must belong to one book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS price_series_same_book
BEFORE INSERT ON price_series
WHEN NOT EXISTS (
  SELECT 1
  FROM commodities b
  JOIN commodities q ON q.id = NEW.quote_commodity_id
  WHERE b.id = NEW.base_commodity_id
    AND b.book_id = NEW.book_id
    AND q.book_id = NEW.book_id
)
BEGIN
  SELECT RAISE(ABORT, 'price series commodities must belong to one book');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS price_observations_same_book
BEFORE INSERT ON price_observations
WHEN NOT EXISTS (
  SELECT 1
  FROM price_series ps
  JOIN commodities c ON c.id = NEW.base_commodity_id
  JOIN commodities q ON q.id = NEW.quote_commodity_id
  WHERE ps.id = NEW.series_id
    AND ps.book_id = NEW.book_id
    AND ps.base_commodity_id = NEW.base_commodity_id
    AND ps.quote_commodity_id = NEW.quote_commodity_id
    AND c.book_id = NEW.book_id
    AND q.book_id = NEW.book_id
)
BEGIN
  SELECT RAISE(ABORT, 'price observation series and commodities must belong to one book');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS price_observations_same_book;
DROP TRIGGER IF EXISTS price_series_same_book;
DROP TRIGGER IF EXISTS pricing_source_assignments_same_book;
DROP TRIGGER IF EXISTS investment_lot_events_same_book;
DROP TRIGGER IF EXISTS investment_lots_same_book;
DROP TRIGGER IF EXISTS investment_instruments_commodity_must_be_security;
DROP TRIGGER IF EXISTS investment_instrument_versions_no_delete;
DROP TRIGGER IF EXISTS investment_instrument_versions_no_update;

DROP INDEX IF EXISTS investment_automation_rules_lookup_idx;
DROP TABLE IF EXISTS investment_automation_rules;
DROP INDEX IF EXISTS investment_event_suggestions_book_idx;
DROP TABLE IF EXISTS investment_event_suggestions;
DROP INDEX IF EXISTS investment_provider_events_book_idx;
DROP INDEX IF EXISTS investment_provider_events_source_event_idx;
DROP TABLE IF EXISTS investment_provider_events;
DROP INDEX IF EXISTS investment_lot_events_lot_idx;
DROP TABLE IF EXISTS investment_lot_events;
DROP INDEX IF EXISTS investment_lots_position_idx;
DROP TABLE IF EXISTS investment_lots;
DROP INDEX IF EXISTS dividend_defaults_lookup_idx;
DROP TABLE IF EXISTS dividend_defaults;
DROP INDEX IF EXISTS cost_basis_profiles_default_idx;
DROP TABLE IF EXISTS cost_basis_profiles;
DROP TABLE IF EXISTS pricing_refresh_state;
DROP INDEX IF EXISTS market_data_ingest_items_run_idx;
DROP TABLE IF EXISTS market_data_ingest_items;
DROP INDEX IF EXISTS market_data_ingest_runs_book_idx;
DROP TABLE IF EXISTS market_data_ingest_runs;
DROP INDEX IF EXISTS price_observations_provider_event_idx;
DROP INDEX IF EXISTS price_observations_lookup_idx;
DROP TABLE IF EXISTS price_observations;
DROP INDEX IF EXISTS price_series_provider_idx;
DROP INDEX IF EXISTS price_series_lookup_idx;
DROP TABLE IF EXISTS price_series;
DROP INDEX IF EXISTS pricing_source_assignments_lookup_idx;
DROP TABLE IF EXISTS pricing_source_assignments;
DROP TABLE IF EXISTS pricing_policies;
DROP INDEX IF EXISTS instrument_search_cache_query_idx;
DROP TABLE IF EXISTS instrument_search_cache;
DROP INDEX IF EXISTS instrument_source_links_active_provider_idx;
DROP INDEX IF EXISTS instrument_source_links_lookup_idx;
DROP TABLE IF EXISTS instrument_source_links;
DROP INDEX IF EXISTS investment_instrument_versions_symbol_idx;
DROP INDEX IF EXISTS investment_instrument_versions_instrument_seq_idx;
DROP VIEW IF EXISTS current_investment_instrument_versions;
DROP TABLE IF EXISTS investment_instrument_versions;
DROP INDEX IF EXISTS investment_instruments_book_idx;
DROP TABLE IF EXISTS investment_instruments;
DROP TABLE IF EXISTS market_data_sources;
