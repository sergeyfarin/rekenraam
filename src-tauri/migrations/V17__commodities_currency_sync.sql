-- V17: Sync full currency list into commodities table

-- Insert missing currencies from the currencies seed table into commodities
INSERT OR IGNORE INTO commodities
  (book_id, kind, symbol, display_symbol, name, scale, is_active, is_default, created_at, updated_at)
SELECT
  c.book_id,
  'currency',
  c.code,
  c.symbol,
  c.name,
  2,
  CASE WHEN c.code IN ('USD', 'EUR', 'GBP') THEN 1 ELSE 0 END,
  CASE WHEN c.code = 'USD' THEN 1 ELSE 0 END,
  strftime('%Y-%m-%dT%H:%M:%fZ','now'),
  strftime('%Y-%m-%dT%H:%M:%fZ','now')
FROM currencies c
WHERE c.book_id = 1;

-- Ensure core currencies are active
UPDATE commodities
SET is_active = 1, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE book_id = 1 AND kind = 'currency' AND symbol IN ('USD', 'EUR', 'GBP');

-- Ensure USD is the default currency if no default set
UPDATE commodities
SET is_default = 1, is_active = 1, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE book_id = 1 AND kind = 'currency' AND symbol = 'USD'
  AND NOT EXISTS (SELECT 1 FROM commodities WHERE book_id = 1 AND kind = 'currency' AND is_default = 1);
