CREATE TABLE IF NOT EXISTS accounts (
    id BIGSERIAL PRIMARY KEY,
    book_id BIGINT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    parent_id BIGINT REFERENCES accounts(id) ON DELETE SET NULL,
    account_type TEXT NOT NULL CHECK (account_type IN ('asset', 'liability', 'income', 'expense', 'equity')),
    name TEXT NOT NULL,
    currency_code CHAR(3) NOT NULL,
    is_closed BOOLEAN NOT NULL DEFAULT FALSE,
    is_hidden BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_accounts_book_id ON accounts(book_id);
CREATE INDEX IF NOT EXISTS idx_accounts_book_name ON accounts(book_id, name);

INSERT INTO accounts (book_id, parent_id, account_type, name, currency_code, is_closed, is_hidden)
SELECT b.id, NULL, seeded.account_type, seeded.name, seeded.currency_code, FALSE, FALSE
FROM books b
CROSS JOIN (
    VALUES
        ('asset', 'Cash', 'USD'),
        ('asset', 'Checking Account', 'USD')
) AS seeded(account_type, name, currency_code)
WHERE b.slug = 'personal'
  AND NOT EXISTS (
      SELECT 1
      FROM accounts existing
      WHERE existing.book_id = b.id
        AND existing.name = seeded.name
  );

INSERT INTO app_schema_state (id, schema_version)
VALUES (TRUE, '0002_accounts')
ON CONFLICT (id) DO UPDATE
SET schema_version = EXCLUDED.schema_version,
    migrated_at = NOW();