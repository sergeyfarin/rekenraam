-- V16: Track derived vs downloaded daily FX rates

ALTER TABLE fx_rates_daily ADD COLUMN is_derived INTEGER NOT NULL DEFAULT 0;
ALTER TABLE fx_rates_daily ADD COLUMN derived_via_currency_id INTEGER;

CREATE INDEX IF NOT EXISTS idx_fx_daily_derived ON fx_rates_daily(is_derived);
CREATE INDEX IF NOT EXISTS idx_fx_daily_derived_via ON fx_rates_daily(derived_via_currency_id);
