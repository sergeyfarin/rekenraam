-- +goose Up
-- #485 second review: a link names its line, which stays its identity across
-- versions. As evidence it also keeps the posting version the owner linked,
-- so an export can state the linked facts after the target is voided or
-- deleted and no longer in ledger.csv. New links record it as they are
-- written. Existing links (no release has them) are backfilled only where the
-- order is certain, never from timestamps, which tie within a second: audit
-- event ids follow commit order on the single writer, so the version linked
-- is the line's latest version whose change event precedes the link's own
-- event. A transaction with a version lacking a change event is not
-- ordered, and its links stay without evidence (NULL, exported as
-- unrecorded) rather than with a guess.
ALTER TABLE import_posting_links ADD COLUMN linked_posting_version_id INTEGER
  REFERENCES posting_versions(id) ON DELETE RESTRICT;

-- The backfill is the one permitted update of these append-only rows.
DROP TRIGGER import_posting_links_no_update;

UPDATE import_posting_links SET linked_posting_version_id = (
  SELECT candidate.id FROM posting_versions candidate
  JOIN transaction_versions version ON version.id = candidate.transaction_version_id
  WHERE candidate.posting_line_id = import_posting_links.posting_line_id
    AND version.change_audit_event_id < import_posting_links.created_audit_event_id
  ORDER BY version.change_audit_event_id DESC, candidate.id DESC LIMIT 1
)
WHERE NOT EXISTS (
  SELECT 1 FROM transaction_versions unordered
  WHERE unordered.transaction_id = import_posting_links.transaction_id
    AND unordered.change_audit_event_id IS NULL
);

-- +goose StatementBegin
CREATE TRIGGER import_posting_links_no_update
BEFORE UPDATE ON import_posting_links
BEGIN
  SELECT RAISE(ABORT, 'import_posting_links rows are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER import_posting_links_evidence
BEFORE INSERT ON import_posting_links
BEGIN
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1 FROM posting_versions pv
    WHERE pv.id = NEW.linked_posting_version_id AND pv.posting_line_id = NEW.posting_line_id
  ) THEN RAISE(ABORT, 'an import posting link records the posting version of its line it linked') END;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER import_posting_links_evidence;
ALTER TABLE import_posting_links DROP COLUMN linked_posting_version_id;
