-- +goose Up
CREATE TABLE investment_allocation_revisions (
    id INTEGER PRIMARY KEY,
    book_id INTEGER NOT NULL REFERENCES books(id),
    saved_at TEXT NOT NULL,
    created_by_user_id INTEGER NOT NULL REFERENCES users(id),
    audit_event_id INTEGER NOT NULL REFERENCES audit_events(id),
    account_ids_json TEXT NOT NULL CHECK (json_valid(account_ids_json) AND json_type(account_ids_json) = 'array'),
    targets_json TEXT NOT NULL CHECK (json_valid(targets_json) AND json_type(targets_json) = 'array')
);
CREATE INDEX investment_allocation_revisions_book ON investment_allocation_revisions(book_id, id DESC);
-- +goose StatementBegin
CREATE TRIGGER investment_allocation_revisions_no_update BEFORE UPDATE ON investment_allocation_revisions
BEGIN SELECT RAISE(ABORT, 'allocation revisions are immutable'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER investment_allocation_revisions_no_delete BEFORE DELETE ON investment_allocation_revisions
BEGIN SELECT RAISE(ABORT, 'allocation revisions are immutable'); END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER investment_allocation_revisions_no_delete;
DROP TRIGGER investment_allocation_revisions_no_update;
DROP TABLE investment_allocation_revisions;
