-- +goose Up
-- The JSON snapshots retain durable identities just like foreign keys. Guard
-- every revision, including references removed from the current selection.
-- +goose StatementBegin
CREATE TRIGGER allocation_history_account_no_delete BEFORE DELETE ON accounts
WHEN EXISTS (
    SELECT 1 FROM investment_allocation_revisions r, json_each(r.account_ids_json) j
    WHERE r.book_id = OLD.book_id AND j.value = OLD.id
)
BEGIN SELECT RAISE(ABORT, 'account is referenced by allocation history'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER allocation_history_commodity_no_delete BEFORE DELETE ON commodities
WHEN EXISTS (
    SELECT 1 FROM investment_allocation_revisions r, json_each(r.targets_json) j
    WHERE r.book_id = OLD.book_id AND json_extract(j.value, '$.commodity_id') = OLD.id
)
BEGIN SELECT RAISE(ABORT, 'commodity is referenced by allocation history'); END;
-- +goose StatementEnd


-- +goose Down
DROP TRIGGER allocation_history_commodity_no_delete;
DROP TRIGGER allocation_history_account_no_delete;
