-- +goose Up
ALTER TABLE books ADD COLUMN updated_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT;

UPDATE books
SET updated_by_user_id = owner_user_id
WHERE updated_by_user_id IS NULL;

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS books_default_currency_must_exist_on_insert
BEFORE INSERT ON books
WHEN NEW.default_currency_commodity_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM commodities
    WHERE id = NEW.default_currency_commodity_id
      AND book_id = NEW.id
      AND kind = 'currency'
  )
BEGIN
  SELECT RAISE(ABORT, 'book default currency must reference a currency in the same book');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS books_default_currency_must_exist_on_insert;
ALTER TABLE books DROP COLUMN updated_by_user_id;
