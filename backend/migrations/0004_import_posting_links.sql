-- +goose Up
-- #485: a statement row that matches a posting already in the ledger (a
-- transfer entered by hand, say) commits as evidence, not as a journal: its
-- import identity links the row to that posting line. A line is stable
-- across versions, so the link survives a non-financial edit; it never names
-- a version. Each line is linked to at most one statement row, and the link
-- is append-only like the identity it belongs to.
CREATE TABLE import_posting_links (
  identity_id INTEGER PRIMARY KEY REFERENCES import_commit_identities(id) ON DELETE RESTRICT,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE RESTRICT,
  posting_line_id INTEGER NOT NULL UNIQUE REFERENCES posting_lines(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER NOT NULL REFERENCES audit_events(id) ON DELETE RESTRICT,
  created_at TEXT NOT NULL
);

CREATE INDEX import_posting_links_transaction_idx ON import_posting_links (transaction_id);

-- +goose StatementBegin
CREATE TRIGGER import_posting_links_same_book
BEFORE INSERT ON import_posting_links
BEGIN
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1 FROM import_commit_identities i
    JOIN posting_lines line ON line.id = NEW.posting_line_id
    JOIN transactions t ON t.id = NEW.transaction_id
    WHERE i.id = NEW.identity_id AND i.book_id = NEW.book_id
      AND line.book_id = NEW.book_id AND line.transaction_id = NEW.transaction_id
      AND t.book_id = NEW.book_id
  ) THEN RAISE(ABORT, 'import posting link must name a line of its transaction in the identity book') END;
  SELECT CASE WHEN EXISTS (
    SELECT 1 FROM import_commit_identity_effects e WHERE e.identity_id = NEW.identity_id
  ) THEN RAISE(ABORT, 'an import identity either links a posting or records effects') END;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER import_posting_links_no_update
BEFORE UPDATE ON import_posting_links
BEGIN
  SELECT RAISE(ABORT, 'import_posting_links rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER import_posting_links_no_delete
BEFORE DELETE ON import_posting_links
BEGIN
  SELECT RAISE(ABORT, 'import_posting_links rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER import_commit_identity_effects_not_linked
BEFORE INSERT ON import_commit_identity_effects
BEGIN
  SELECT CASE WHEN EXISTS (
    SELECT 1 FROM import_posting_links l WHERE l.identity_id = NEW.identity_id
  ) THEN RAISE(ABORT, 'an import identity either links a posting or records effects') END;
END;
-- +goose StatementEnd

-- A committed row's result is its identity's effects or, for a linked row,
-- its posting link; the transaction summary names the first effect's
-- transaction or the linked one.
DROP TRIGGER import_staged_rows_identity_same_book;

-- +goose StatementBegin
CREATE TRIGGER import_staged_rows_identity_same_book
BEFORE UPDATE OF commit_status, committed_identity_id, committed_transaction_id ON import_staged_rows
BEGIN
  SELECT CASE WHEN NEW.commit_status = 'committed' AND NOT EXISTS (
    SELECT 1 FROM import_commit_identities i
    WHERE i.id = NEW.committed_identity_id AND i.book_id = NEW.book_id
      AND i.source_fingerprint = NEW.dedupe_fingerprint
  ) THEN RAISE(ABORT, 'staged row identity must match book and fingerprint') END;
  SELECT CASE WHEN NEW.commit_status = 'committed' AND NOT EXISTS (
    SELECT 1 FROM import_commit_identity_effects e WHERE e.identity_id = NEW.committed_identity_id
  ) AND NOT EXISTS (
    SELECT 1 FROM import_posting_links l WHERE l.identity_id = NEW.committed_identity_id
  ) THEN RAISE(ABORT, 'committed staged row requires identity effects') END;
  SELECT CASE WHEN NEW.commit_status = 'committed' AND NEW.committed_transaction_id IS NOT COALESCE((
    SELECT e.transaction_id FROM import_commit_identity_effects e
    WHERE e.identity_id = NEW.committed_identity_id AND e.transaction_id IS NOT NULL
    ORDER BY e.effect_seq LIMIT 1
  ), (
    SELECT l.transaction_id FROM import_posting_links l WHERE l.identity_id = NEW.committed_identity_id
  )) THEN RAISE(ABORT, 'staged row transaction summary must match first effect') END;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER import_staged_rows_identity_same_book;

-- +goose StatementBegin
CREATE TRIGGER import_staged_rows_identity_same_book
BEFORE UPDATE OF commit_status, committed_identity_id, committed_transaction_id ON import_staged_rows
BEGIN
  SELECT CASE WHEN NEW.commit_status = 'committed' AND NOT EXISTS (
    SELECT 1 FROM import_commit_identities i
    WHERE i.id = NEW.committed_identity_id AND i.book_id = NEW.book_id
      AND i.source_fingerprint = NEW.dedupe_fingerprint
  ) THEN RAISE(ABORT, 'staged row identity must match book and fingerprint') END;
  SELECT CASE WHEN NEW.commit_status = 'committed' AND NOT EXISTS (
    SELECT 1 FROM import_commit_identity_effects e WHERE e.identity_id = NEW.committed_identity_id
  ) THEN RAISE(ABORT, 'committed staged row requires identity effects') END;
  SELECT CASE WHEN NEW.commit_status = 'committed' AND NEW.committed_transaction_id IS NOT (
    SELECT e.transaction_id FROM import_commit_identity_effects e
    WHERE e.identity_id = NEW.committed_identity_id AND e.transaction_id IS NOT NULL
    ORDER BY e.effect_seq LIMIT 1
  ) THEN RAISE(ABORT, 'staged row transaction summary must match first effect') END;
END;
-- +goose StatementEnd

DROP TRIGGER import_commit_identity_effects_not_linked;
DROP TRIGGER import_posting_links_no_delete;
DROP TRIGGER import_posting_links_no_update;
DROP TRIGGER import_posting_links_same_book;
DROP TABLE import_posting_links;
