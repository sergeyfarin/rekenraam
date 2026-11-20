-- +goose Up
INSERT INTO market_data_sources (code, name, kind, provider_key, status, metadata_json, created_at)
VALUES ('massive', 'Massive', 'provider', 'massive', 'active',
        '{"capabilities":["splits"],"requires_secret":true,"coverage":"us"}', '0001-01-01T00:00:00Z');

-- +goose Down
DELETE FROM market_data_sources WHERE code = 'massive';
