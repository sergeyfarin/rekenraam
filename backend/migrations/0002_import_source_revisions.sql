-- +goose Up
-- Each accepted provider revision keeps its own staged payload and links the
-- original dedupe identity to the correcting investment operation. The source
-- identity effects continue to name the first committed fill.
CREATE TABLE import_source_revisions (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  identity_id INTEGER NOT NULL REFERENCES import_commit_identities(id) ON DELETE RESTRICT,
  staged_row_id INTEGER NOT NULL UNIQUE REFERENCES import_staged_rows(id) ON DELETE RESTRICT,
  source_operation_id INTEGER NOT NULL REFERENCES investment_operations(id) ON DELETE RESTRICT,
  correction_operation_id INTEGER NOT NULL UNIQUE REFERENCES investment_operations(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER NOT NULL REFERENCES audit_events(id) ON DELETE RESTRICT,
  created_at TEXT NOT NULL
);

CREATE INDEX import_source_revisions_identity_idx
  ON import_source_revisions (identity_id, id);

-- +goose StatementBegin
CREATE TRIGGER import_source_revisions_same_book
BEFORE INSERT ON import_source_revisions
BEGIN
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1 FROM import_commit_identities identity_row
    JOIN import_staged_rows staged ON staged.id = NEW.staged_row_id
    WHERE identity_row.id = NEW.identity_id AND identity_row.book_id = NEW.book_id
      AND staged.book_id = NEW.book_id
      AND staged.dedupe_fingerprint = identity_row.dedupe_fingerprint
      AND staged.commit_status = 'committed'
      AND staged.committed_identity_id = identity_row.id
  ) THEN RAISE(ABORT, 'source revision row must belong to committed identity') END;
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1 FROM import_commit_identity_effects effect
    JOIN investment_operations source ON source.id = effect.operation_id
    WHERE effect.identity_id = NEW.identity_id
      AND source.id = NEW.source_operation_id AND source.book_id = NEW.book_id
  ) THEN RAISE(ABORT, 'source revision must name original identity operation') END;
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1 FROM investment_operations correction
    JOIN audit_events audit ON audit.id = correction.created_audit_event_id
    WHERE correction.id = NEW.correction_operation_id
      AND correction.book_id = NEW.book_id
      AND correction.correction_mode IN ('replace', 'reverse')
      AND audit.id = NEW.created_audit_event_id AND audit.book_id = NEW.book_id
  ) THEN RAISE(ABORT, 'source revision must name audited correction operation') END;
  SELECT CASE WHEN NOT EXISTS (
    WITH RECURSIVE ancestors(id, parent_id) AS (
      SELECT id, correction_of_operation_id FROM investment_operations
      WHERE id = NEW.correction_operation_id AND book_id = NEW.book_id
      UNION ALL
      SELECT parent.id, parent.correction_of_operation_id
      FROM investment_operations parent JOIN ancestors child ON parent.id = child.parent_id
      WHERE parent.book_id = NEW.book_id
    )
    SELECT 1 FROM ancestors WHERE id = NEW.source_operation_id
  ) THEN RAISE(ABORT, 'source revision correction must descend from source operation') END;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER import_source_revisions_no_update
BEFORE UPDATE ON import_source_revisions
BEGIN
  SELECT RAISE(ABORT, 'import source revision is immutable');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER import_source_revisions_no_delete
BEFORE DELETE ON import_source_revisions
BEGIN
  SELECT RAISE(ABORT, 'import source revision is immutable');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS import_source_revisions_no_delete;
DROP TRIGGER IF EXISTS import_source_revisions_no_update;
DROP TRIGGER IF EXISTS import_source_revisions_same_book;
DROP INDEX IF EXISTS import_source_revisions_identity_idx;
DROP TABLE IF EXISTS import_source_revisions;
