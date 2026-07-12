-- +goose Up

-- A failed online-import attempt may create an instrument before discovering
-- that its trade cannot post. Unreferenced structural records may be removed
-- as compensation, but an instrument with any financial, provider, price,
-- default, source-link, or connection-holding reference remains append-only.
DROP TRIGGER IF EXISTS investment_instrument_versions_no_delete;
DROP TRIGGER IF EXISTS commodity_versions_no_delete;

-- +goose StatementBegin
CREATE TRIGGER investment_instrument_versions_no_delete
BEFORE DELETE ON investment_instrument_versions
WHEN EXISTS (
  SELECT 1
  FROM investment_instruments ii
  WHERE ii.id = OLD.instrument_id
    AND (
      EXISTS (SELECT 1 FROM investment_lots lot WHERE lot.commodity_id = ii.commodity_id)
      OR EXISTS (SELECT 1 FROM posting_versions pv WHERE pv.commodity_id = ii.commodity_id)
      OR EXISTS (SELECT 1 FROM import_connection_holdings ich WHERE ich.commodity_id = ii.commodity_id)
      OR EXISTS (SELECT 1 FROM dividend_defaults dd WHERE dd.commodity_id = ii.commodity_id)
      OR EXISTS (SELECT 1 FROM price_series ps WHERE ps.base_commodity_id = ii.commodity_id OR ps.quote_commodity_id = ii.commodity_id)
      OR EXISTS (SELECT 1 FROM instrument_source_links isl WHERE isl.instrument_id = ii.id)
      OR EXISTS (SELECT 1 FROM investment_provider_events ipe WHERE ipe.instrument_id = ii.id)
      OR EXISTS (SELECT 1 FROM investment_event_suggestions ies WHERE ies.instrument_id = ii.id)
      OR EXISTS (SELECT 1 FROM investment_automation_rules iar WHERE iar.instrument_id = ii.id)
    )
)
BEGIN
  SELECT RAISE(ABORT, 'investment_instrument_versions rows are append-only once referenced');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER commodity_versions_no_delete
BEFORE DELETE ON commodity_versions
WHEN EXISTS (
  SELECT 1
  FROM commodities c
  WHERE c.id = OLD.commodity_id
    AND (
      c.kind != 'security'
      OR EXISTS (SELECT 1 FROM posting_versions pv WHERE pv.commodity_id = c.id)
      OR EXISTS (SELECT 1 FROM investment_lots lot WHERE lot.commodity_id = c.id OR lot.cost_commodity_id = c.id)
      OR EXISTS (SELECT 1 FROM import_connection_holdings ich WHERE ich.commodity_id = c.id)
      OR EXISTS (SELECT 1 FROM dividend_defaults dd WHERE dd.commodity_id = c.id)
      OR EXISTS (SELECT 1 FROM account_versions av WHERE av.default_commodity_id = c.id)
      OR EXISTS (SELECT 1 FROM price_series ps WHERE ps.base_commodity_id = c.id OR ps.quote_commodity_id = c.id)
      OR EXISTS (SELECT 1 FROM price_observations po WHERE po.base_commodity_id = c.id OR po.quote_commodity_id = c.id)
    )
)
BEGIN
  SELECT RAISE(ABORT, 'commodity_versions rows are append-only once referenced');
END;
-- +goose StatementEnd

-- +goose Down

DROP TRIGGER IF EXISTS investment_instrument_versions_no_delete;
DROP TRIGGER IF EXISTS commodity_versions_no_delete;

-- +goose StatementBegin
CREATE TRIGGER investment_instrument_versions_no_delete
BEFORE DELETE ON investment_instrument_versions
BEGIN
  SELECT RAISE(ABORT, 'investment_instrument_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER commodity_versions_no_delete
BEFORE DELETE ON commodity_versions
BEGIN
  SELECT RAISE(ABORT, 'commodity_versions rows are append-only');
END;
-- +goose StatementEnd
