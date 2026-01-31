-- V14: Add display_symbol column for currency symbols (e.g., $, €, £, ¥)

ALTER TABLE commodities ADD COLUMN display_symbol TEXT;

-- Populate display symbols for pre-seeded currencies
UPDATE commodities SET display_symbol = '$' WHERE symbol = 'USD' AND kind = 'currency';
UPDATE commodities SET display_symbol = '€' WHERE symbol = 'EUR' AND kind = 'currency';
UPDATE commodities SET display_symbol = '£' WHERE symbol = 'GBP' AND kind = 'currency';
UPDATE commodities SET display_symbol = '¥' WHERE symbol = 'JPY' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'Fr.' WHERE symbol = 'CHF' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'C$' WHERE symbol = 'CAD' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'A$' WHERE symbol = 'AUD' AND kind = 'currency';
UPDATE commodities SET display_symbol = '¥' WHERE symbol = 'CNY' AND kind = 'currency';
UPDATE commodities SET display_symbol = '₹' WHERE symbol = 'INR' AND kind = 'currency';
UPDATE commodities SET display_symbol = '$' WHERE symbol = 'MXN' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'R$' WHERE symbol = 'BRL' AND kind = 'currency';
UPDATE commodities SET display_symbol = '₩' WHERE symbol = 'KRW' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'S$' WHERE symbol = 'SGD' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'HK$' WHERE symbol = 'HKD' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'kr' WHERE symbol = 'NOK' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'kr' WHERE symbol = 'SEK' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'kr' WHERE symbol = 'DKK' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'NZ$' WHERE symbol = 'NZD' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'R' WHERE symbol = 'ZAR' AND kind = 'currency';
UPDATE commodities SET display_symbol = '₽' WHERE symbol = 'RUB' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'zł' WHERE symbol = 'PLN' AND kind = 'currency';
UPDATE commodities SET display_symbol = '₺' WHERE symbol = 'TRY' AND kind = 'currency';
UPDATE commodities SET display_symbol = '฿' WHERE symbol = 'THB' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'Rp' WHERE symbol = 'IDR' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'RM' WHERE symbol = 'MYR' AND kind = 'currency';
UPDATE commodities SET display_symbol = '₱' WHERE symbol = 'PHP' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'Kč' WHERE symbol = 'CZK' AND kind = 'currency';
UPDATE commodities SET display_symbol = '₪' WHERE symbol = 'ILS' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'د.إ' WHERE symbol = 'AED' AND kind = 'currency';
UPDATE commodities SET display_symbol = '﷼' WHERE symbol = 'SAR' AND kind = 'currency';
UPDATE commodities SET display_symbol = 'NT$' WHERE symbol = 'TWD' AND kind = 'currency';
