-- V13: Currency management and FX rates
-- Add active/default flags to commodities for currency management

ALTER TABLE commodities ADD COLUMN is_active INTEGER NOT NULL DEFAULT 1;
ALTER TABLE commodities ADD COLUMN is_default INTEGER NOT NULL DEFAULT 0;

-- Create index for active currencies
CREATE INDEX IF NOT EXISTS idx_commodities_active ON commodities(book_id, kind, is_active);

-- Daily FX rates (market rates)
CREATE TABLE IF NOT EXISTS fx_rates_daily (
  id                 INTEGER PRIMARY KEY,
  book_id            INTEGER NOT NULL,
  from_currency_id   INTEGER NOT NULL,
  to_currency_id     INTEGER NOT NULL,
  rate_date          TEXT NOT NULL,
  rate               REAL NOT NULL,
  source             TEXT,
  created_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(from_currency_id) REFERENCES commodities(id) ON DELETE CASCADE,
  FOREIGN KEY(to_currency_id) REFERENCES commodities(id) ON DELETE CASCADE,
  UNIQUE(book_id, from_currency_id, to_currency_id, rate_date)
);

CREATE INDEX IF NOT EXISTS idx_fx_daily_book_date ON fx_rates_daily(book_id, rate_date);
CREATE INDEX IF NOT EXISTS idx_fx_daily_currencies ON fx_rates_daily(from_currency_id, to_currency_id);

-- Official FX rates (tax authority rates - monthly/yearly)
CREATE TABLE IF NOT EXISTS fx_rates_official (
  id                 INTEGER PRIMARY KEY,
  book_id            INTEGER NOT NULL,
  from_currency_id   INTEGER NOT NULL,
  to_currency_id     INTEGER NOT NULL,
  period_type        TEXT NOT NULL CHECK (period_type IN ('monthly', 'yearly')),
  period_year        INTEGER NOT NULL,
  period_month       INTEGER,  -- NULL for yearly rates, 1-12 for monthly
  rate               REAL NOT NULL,
  source_name        TEXT NOT NULL,  -- e.g., "IRS", "HMRC", "ECB", "Belastingdienst"
  source_url         TEXT,
  source_date        TEXT,  -- Date the rate was published
  notes              TEXT,
  created_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  FOREIGN KEY(from_currency_id) REFERENCES commodities(id) ON DELETE CASCADE,
  FOREIGN KEY(to_currency_id) REFERENCES commodities(id) ON DELETE CASCADE,
  UNIQUE(book_id, from_currency_id, to_currency_id, period_type, period_year, period_month)
);

CREATE INDEX IF NOT EXISTS idx_fx_official_book ON fx_rates_official(book_id);
CREATE INDEX IF NOT EXISTS idx_fx_official_period ON fx_rates_official(period_type, period_year, period_month);
CREATE INDEX IF NOT EXISTS idx_fx_official_currencies ON fx_rates_official(from_currency_id, to_currency_id);

-- Tax authority sources reference table
CREATE TABLE IF NOT EXISTS fx_rate_sources (
  id           INTEGER PRIMARY KEY,
  book_id      INTEGER NOT NULL,
  name         TEXT NOT NULL,
  country_code TEXT,
  website_url  TEXT,
  notes        TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  FOREIGN KEY(book_id) REFERENCES books(id) ON DELETE CASCADE,
  UNIQUE(book_id, name)
);

-- Seed common FX rate sources
INSERT OR IGNORE INTO fx_rate_sources (book_id, name, country_code, website_url, notes) VALUES
  (1, 'ECB', 'EU', 'https://www.ecb.europa.eu/stats/exchange/eurofxref/', 'European Central Bank reference rates'),
  (1, 'IRS', 'US', 'https://www.irs.gov/individuals/international-taxpayers/yearly-average-currency-exchange-rates', 'US Internal Revenue Service yearly average rates'),
  (1, 'HMRC', 'GB', 'https://www.gov.uk/government/collections/exchange-rates-for-customs-and-vat', 'UK HM Revenue & Customs rates'),
  (1, 'Belastingdienst', 'NL', 'https://www.belastingdienst.nl/wps/wcm/connect/nl/koerslijst/', 'Dutch Tax Authority rates'),
  (1, 'Federal Reserve', 'US', 'https://www.federalreserve.gov/releases/h10/current/', 'US Federal Reserve H.10 rates'),
  (1, 'Bank of Canada', 'CA', 'https://www.bankofcanada.ca/rates/exchange/', 'Bank of Canada exchange rates'),
  (1, 'RBA', 'AU', 'https://www.rba.gov.au/statistics/frequency/exchange-rates.html', 'Reserve Bank of Australia rates'),
  (1, 'SNB', 'CH', 'https://www.snb.ch/en/iabout/stat/statrep/id/current_interest_exchange_rates', 'Swiss National Bank rates');

-- Set USD as default currency if it exists
UPDATE commodities 
SET is_default = 1 
WHERE book_id = 1 AND symbol = 'USD' AND kind = 'currency';

-- Pre-seed major world currencies as active commodities
INSERT OR IGNORE INTO commodities (book_id, kind, symbol, name, scale, is_active, is_default, created_at, updated_at) VALUES
  (1, 'currency', 'EUR', 'Euro', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'GBP', 'British Pound Sterling', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'JPY', 'Japanese Yen', 0, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'CHF', 'Swiss Franc', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'CAD', 'Canadian Dollar', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'AUD', 'Australian Dollar', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'CNY', 'Chinese Yuan', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'INR', 'Indian Rupee', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'MXN', 'Mexican Peso', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'BRL', 'Brazilian Real', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'KRW', 'South Korean Won', 0, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'SGD', 'Singapore Dollar', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'HKD', 'Hong Kong Dollar', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'NOK', 'Norwegian Krone', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'SEK', 'Swedish Krona', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'DKK', 'Danish Krone', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'NZD', 'New Zealand Dollar', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'ZAR', 'South African Rand', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'RUB', 'Russian Ruble', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'PLN', 'Polish Zloty', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'TRY', 'Turkish Lira', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'THB', 'Thai Baht', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'IDR', 'Indonesian Rupiah', 0, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'MYR', 'Malaysian Ringgit', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'PHP', 'Philippine Peso', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'CZK', 'Czech Koruna', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'ILS', 'Israeli New Shekel', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'AED', 'UAE Dirham', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'SAR', 'Saudi Riyal', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (1, 'currency', 'TWD', 'Taiwan Dollar', 2, 1, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'));

-- Ensure USD is marked as active
UPDATE commodities SET is_active = 1 WHERE book_id = 1 AND symbol = 'USD' AND kind = 'currency';
