-- +goose Up
-- #485 review: a mistaken link must be repairable without losing what was
-- recorded. A link becomes one immutable row per association; unlinking
-- appends a retirement, and reassigning retires the current link and appends
-- its successor (replaces_link_id), both under one audit event. A row has at
-- most one active link and a posting line at most one active link; retired
-- links stay as evidence.

DROP TRIGGER import_staged_rows_identity_same_book;
DROP TRIGGER import_commit_identity_effects_not_linked;
DROP TRIGGER import_posting_links_no_delete;
DROP TRIGGER import_posting_links_no_update;
DROP TRIGGER import_posting_links_same_book;
DROP INDEX import_posting_links_transaction_idx;

CREATE TABLE import_posting_links_v2 (
  id INTEGER PRIMARY KEY,
  identity_id INTEGER NOT NULL REFERENCES import_commit_identities(id) ON DELETE RESTRICT,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE RESTRICT,
  posting_line_id INTEGER NOT NULL REFERENCES posting_lines(id) ON DELETE RESTRICT,
  replaces_link_id INTEGER UNIQUE REFERENCES import_posting_links_v2(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER NOT NULL REFERENCES audit_events(id) ON DELETE RESTRICT,
  created_at TEXT NOT NULL
);

INSERT INTO import_posting_links_v2 (identity_id, book_id, transaction_id, posting_line_id, created_audit_event_id, created_at)
SELECT identity_id, book_id, transaction_id, posting_line_id, created_audit_event_id, created_at
FROM import_posting_links ORDER BY identity_id;

DROP TABLE import_posting_links;
ALTER TABLE import_posting_links_v2 RENAME TO import_posting_links;

CREATE INDEX import_posting_links_identity_idx ON import_posting_links (identity_id);
CREATE INDEX import_posting_links_line_idx ON import_posting_links (posting_line_id);
CREATE INDEX import_posting_links_transaction_idx ON import_posting_links (transaction_id);

CREATE TABLE import_posting_link_retirements (
  link_id INTEGER PRIMARY KEY REFERENCES import_posting_links(id) ON DELETE RESTRICT,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  reason TEXT NOT NULL CHECK (length(trim(reason)) > 0),
  retired_audit_event_id INTEGER NOT NULL REFERENCES audit_events(id) ON DELETE RESTRICT,
  retired_at TEXT NOT NULL
);

CREATE VIEW active_import_posting_links AS
SELECT link.* FROM import_posting_links link
WHERE NOT EXISTS (SELECT 1 FROM import_posting_link_retirements r WHERE r.link_id = link.id);

-- +goose StatementBegin
CREATE TRIGGER import_posting_links_valid
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
  SELECT CASE WHEN EXISTS (
    SELECT 1 FROM active_import_posting_links l
    WHERE l.identity_id = NEW.identity_id OR l.posting_line_id = NEW.posting_line_id
  ) THEN RAISE(ABORT, 'a statement row and a posting line each have at most one active link') END;
  SELECT CASE WHEN NEW.replaces_link_id IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM import_posting_links previous
    JOIN import_posting_link_retirements r ON r.link_id = previous.id
    WHERE previous.id = NEW.replaces_link_id AND previous.identity_id = NEW.identity_id
  ) THEN RAISE(ABORT, 'a replacement link replaces a retired link of the same statement row') END;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER import_posting_link_retirements_valid
BEFORE INSERT ON import_posting_link_retirements
BEGIN
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1 FROM import_posting_links l WHERE l.id = NEW.link_id AND l.book_id = NEW.book_id
  ) THEN RAISE(ABORT, 'a retirement names a link of its book') END;
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
CREATE TRIGGER import_posting_link_retirements_no_update
BEFORE UPDATE ON import_posting_link_retirements
BEGIN
  SELECT RAISE(ABORT, 'import_posting_link_retirements rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER import_posting_link_retirements_no_delete
BEFORE DELETE ON import_posting_link_retirements
BEGIN
  SELECT RAISE(ABORT, 'import_posting_link_retirements rows are append-only');
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

-- A committed staged row records the commit's own result: the first effect's
-- transaction, or the transaction its first link named. A later reassignment
-- appends a link; the row keeps what it committed.
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
    ORDER BY l.id LIMIT 1
  )) THEN RAISE(ABORT, 'staged row transaction summary must match first effect') END;
END;
-- +goose StatementEnd

-- +goose Down
-- Development only: the 0004 shape keeps one link per row, so retired links
-- and their retirements are dropped and each row keeps its active link.
DROP TRIGGER import_staged_rows_identity_same_book;
DROP TRIGGER import_commit_identity_effects_not_linked;
DROP TRIGGER import_posting_link_retirements_no_delete;
DROP TRIGGER import_posting_link_retirements_no_update;
DROP TRIGGER import_posting_links_no_delete;
DROP TRIGGER import_posting_links_no_update;
DROP TRIGGER import_posting_link_retirements_valid;
DROP TRIGGER import_posting_links_valid;
DROP INDEX import_posting_links_identity_idx;
DROP INDEX import_posting_links_line_idx;
DROP INDEX import_posting_links_transaction_idx;

CREATE TABLE import_posting_links_v1 (
  identity_id INTEGER PRIMARY KEY REFERENCES import_commit_identities(id) ON DELETE RESTRICT,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE RESTRICT,
  posting_line_id INTEGER NOT NULL UNIQUE REFERENCES posting_lines(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER NOT NULL REFERENCES audit_events(id) ON DELETE RESTRICT,
  created_at TEXT NOT NULL
);
INSERT INTO import_posting_links_v1 SELECT identity_id, book_id, transaction_id, posting_line_id, created_audit_event_id, created_at
FROM active_import_posting_links;
DROP VIEW active_import_posting_links;
DROP TABLE import_posting_link_retirements;
DROP TABLE import_posting_links;
ALTER TABLE import_posting_links_v1 RENAME TO import_posting_links;
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
