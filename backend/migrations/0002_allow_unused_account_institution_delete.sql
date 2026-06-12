-- +goose Up
-- +goose StatementBegin
DROP TRIGGER IF EXISTS institution_versions_no_delete;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS institution_versions_no_delete
BEFORE DELETE ON institution_versions
WHEN EXISTS (
  SELECT 1
  FROM account_versions av
  WHERE av.institution_id = OLD.institution_id
)
BEGIN
  SELECT RAISE(ABORT, 'institution_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TRIGGER IF EXISTS account_versions_no_delete;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS account_versions_no_delete
BEFORE DELETE ON account_versions
WHEN NOT (
  (
    OLD.account_class IN ('income', 'expense')
    AND OLD.account_kind = OLD.account_class
    AND json_extract(OLD.metadata_json, '$.category.type') = OLD.account_class
    AND json_extract(OLD.metadata_json, '$.category.is_builtin') = 0
  )
  OR (
    NOT EXISTS (
      SELECT 1
      FROM accounts a
      WHERE a.id = OLD.account_id
        AND a.system_role IS NOT NULL
    )
    AND NOT EXISTS (
      SELECT 1
      FROM posting_versions pv
      WHERE pv.account_id = OLD.account_id
    )
    AND NOT EXISTS (
      SELECT 1
      FROM account_versions child
      WHERE child.parent_account_id = OLD.account_id
    )
  )
)
BEGIN
  SELECT RAISE(ABORT, 'account_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS account_versions_no_delete;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS account_versions_no_delete
BEFORE DELETE ON account_versions
WHEN NOT (
  OLD.account_class IN ('income', 'expense')
  AND OLD.account_kind = OLD.account_class
  AND json_extract(OLD.metadata_json, '$.category.type') = OLD.account_class
  AND json_extract(OLD.metadata_json, '$.category.is_builtin') = 0
)
BEGIN
  SELECT RAISE(ABORT, 'account_versions rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TRIGGER IF EXISTS institution_versions_no_delete;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS institution_versions_no_delete
BEFORE DELETE ON institution_versions
BEGIN
  SELECT RAISE(ABORT, 'institution_versions rows are append-only');
END;
-- +goose StatementEnd
