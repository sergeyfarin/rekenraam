CREATE TABLE IF NOT EXISTS app_schema_state (
    id BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
    schema_version TEXT NOT NULL,
    migrated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO app_schema_state (id, schema_version)
VALUES (TRUE, '0001_initial_schema')
ON CONFLICT (id) DO UPDATE
SET schema_version = EXCLUDED.schema_version,
    migrated_at = NOW();

CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT,
    display_name TEXT NOT NULL,
    is_admin BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS books (
    id BIGSERIAL PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    base_currency_code CHAR(3) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS book_memberships (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_id BIGINT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('owner', 'editor', 'viewer')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, book_id)
);

INSERT INTO books (slug, name, base_currency_code)
VALUES ('personal', 'Personal', 'USD')
ON CONFLICT (slug) DO NOTHING;