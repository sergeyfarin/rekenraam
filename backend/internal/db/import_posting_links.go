package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ImportPostingCandidateRecord is one posting of a current posted
// transaction that a statement row may link to (#485): its line, the day it
// counts on the account (its journal entry's date) and its exact amount.
type ImportPostingCandidateRecord struct {
	TransactionID        int64
	TransactionVersionID int64
	PostingVersionID     int64
	PostingLineID        int64
	LineKey              string
	TransactionDate      string
	TransactionKind      string
	PostedOn             string
	AccountID            int64
	CommodityID          int64
	QuantityValue        string
	QuantityScale        int
	PayeeName            string
	Description          string
	// Linked reports that a statement row already stands for this posting:
	// a posting link names its line, or the transaction is the result of an
	// import identity committed to this same account.
	Linked bool
}

// importPostingCandidateSelect reads postings of current posted, undeleted
// transactions. Its caller filters it by account, commodity and day.
const importPostingCandidateSelect = `
	SELECT t.id, tv.id, pv.id, line.id, line.line_key, tv.transaction_date, tv.transaction_kind,
		entry.entry_date, pv.account_id, pv.commodity_id, pv.quantity_value, pv.quantity_scale,
		COALESCE(tv.payee_name, ''), tv.description,
		(EXISTS (SELECT 1 FROM active_import_posting_links link WHERE link.posting_line_id = line.id)
			OR EXISTS (SELECT 1 FROM import_commit_identity_effects effect
				JOIN import_commit_identities identity_row ON identity_row.id = effect.identity_id
				WHERE effect.transaction_id = t.id AND identity_row.account_id = pv.account_id))
	FROM posting_versions pv
	JOIN journal_entries entry ON entry.id = pv.journal_entry_id
	JOIN current_transaction_versions tv ON tv.id = pv.transaction_version_id
	JOIN transactions t ON t.id = tv.transaction_id
	JOIN posting_lines line ON line.id = pv.posting_line_id
	WHERE t.book_id = ? AND t.deleted_at IS NULL AND tv.status = 'posted'`

func scanImportPostingCandidate(row importStagedRowScanner) (ImportPostingCandidateRecord, error) {
	var record ImportPostingCandidateRecord
	var linked int
	if err := row.Scan(&record.TransactionID, &record.TransactionVersionID, &record.PostingVersionID, &record.PostingLineID, &record.LineKey,
		&record.TransactionDate, &record.TransactionKind, &record.PostedOn, &record.AccountID, &record.CommodityID,
		&record.QuantityValue, &record.QuantityScale, &record.PayeeName, &record.Description, &linked); err != nil {
		return ImportPostingCandidateRecord{}, err
	}
	record.Linked = linked != 0
	return record, nil
}

// ListImportPostingCandidates reads every unlinked posting on accountID in
// commodityID whose posting day is in [fromDate, toDate]. Amounts are
// compared by the caller, scale-aware.
func (r *ImportRepository) ListImportPostingCandidates(ctx context.Context, bookID, accountID, commodityID int64, fromDate, toDate string) ([]ImportPostingCandidateRecord, error) {
	rows, err := r.database.QueryContext(ctx, importPostingCandidateSelect+`
			AND pv.account_id = ? AND pv.commodity_id = ? AND entry.entry_date BETWEEN ? AND ?
		ORDER BY entry.entry_date, t.id, line.id`, bookID, accountID, commodityID, fromDate, toDate)
	if err != nil {
		return nil, fmt.Errorf("read import posting candidates: %w", err)
	}
	defer rows.Close()
	var records []ImportPostingCandidateRecord
	for rows.Next() {
		record, err := scanImportPostingCandidate(rows)
		if err != nil {
			return nil, fmt.Errorf("scan import posting candidate: %w", err)
		}
		if !record.Linked {
			records = append(records, record)
		}
	}
	return records, rows.Err()
}

// LinkImportedPostingParams commits one staged row as a link to an existing
// posting line instead of a new journal.
type LinkImportedPostingParams struct {
	Identity      CreateImportCommitIdentityParams
	RowID         int64
	TransactionID int64
	LineKey       string
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
}

// ErrImportPostingLinkStale means the posting a row was matched to is no
// longer what the owner reviewed: gone, voided, deleted, changed or already
// linked. Nothing was written.
var ErrImportPostingLinkStale = errors.New("matched posting changed after review")

// LinkImportedPosting rereads the matched posting inside the write
// transaction, lets verify judge it against what was reviewed, and only then
// records the identity, its posting link, an audit event and the committed
// row marker together (#485). The read and the write share one transaction,
// so no edit, void or second link can land between them. found is false when
// the line is not a posting of the current posted version.
func (r *ImportRepository) LinkImportedPosting(ctx context.Context, params LinkImportedPostingParams, verify func(candidate ImportPostingCandidateRecord, found bool) error) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin link imported posting: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	candidate, err := scanImportPostingCandidate(tx.QueryRowContext(ctx, importPostingCandidateSelect+`
			AND t.id = ? AND line.line_key = ?`, params.Identity.BookID, params.TransactionID, params.LineKey))
	found := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("reread matched posting: %w", err)
	}
	if err := verify(candidate, found); err != nil {
		return err
	}
	if candidate.Linked {
		return ErrImportPostingLinkStale
	}

	identity := params.Identity
	sourceFingerprint := identity.SourceFingerprint
	if sourceFingerprint == "" {
		sourceFingerprint = identity.DedupeFingerprint
	}
	result, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO import_commit_identities
			(book_id, dedupe_fingerprint, source_fingerprint, source_kind, account_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, identity.BookID, identity.DedupeFingerprint, sourceFingerprint, identity.SourceKind, identity.AccountID, identity.CreatedAt)
	if err != nil {
		return fmt.Errorf("create link commit identity: %w", err)
	}
	if n, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("create link commit identity rows affected: %w", err)
	} else if n == 0 {
		return ErrCommitIdentityConflict
	}
	identityID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("read link commit identity id: %w", err)
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        identity.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    identity.CreatedAt,
		RequestID:     params.RequestID,
		OriginType:    "import",
		Operation:     "import.posting.link",
		Reason:        "statement row matches an existing posting",
	})
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO import_posting_links (identity_id, book_id, transaction_id, posting_line_id, linked_posting_version_id, created_audit_event_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, identityID, identity.BookID, candidate.TransactionID, candidate.PostingLineID, candidate.PostingVersionID, auditEventID, identity.CreatedAt); err != nil {
		return fmt.Errorf("create import posting link: %w", err)
	}
	if err := commitImportStagedRowExec(ctx, tx, CommitImportStagedRowParams{
		RowID:                  params.RowID,
		CommitStatus:           "committed",
		CommittedIdentityID:    sql.NullInt64{Int64: identityID, Valid: true},
		CommittedTransactionID: sql.NullInt64{Int64: candidate.TransactionID, Valid: true},
	}); err != nil {
		return fmt.Errorf("mark linked row committed: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit link imported posting: %w", err)
	}
	committed = true
	return nil
}

// ImportStatementLinkRecord is a committed statement row's current link
// state (#485): its latest link, active or retired, with the row it records.
type ImportStatementLinkRecord struct {
	IdentityID     int64
	LinkID         int64
	TransactionID  int64
	PostingLineID  int64
	LineKey        string
	AccountID      int64
	Active         bool
	LinkedAt       string
	RetiredAt      string
	RetireReason   string
	SourceKind     string
	BatchID        int64
	Filename       string
	NormalizedJSON string
	ResolutionJSON string
}

// importStatementLinkSelect reads each identity's latest link with the
// staged row that first committed it.
const importStatementLinkSelect = `
	SELECT link.identity_id, link.id, link.transaction_id, link.posting_line_id, line.line_key, identity_row.account_id,
		(retirement.link_id IS NULL), link.created_at, COALESCE(retirement.retired_at, ''), COALESCE(retirement.reason, ''),
		identity_row.source_kind, staged.batch_id, batch.original_filename, staged.normalized_json, staged.resolution_json
	FROM import_posting_links link
	JOIN import_commit_identities identity_row ON identity_row.id = link.identity_id
	JOIN posting_lines line ON line.id = link.posting_line_id
	LEFT JOIN import_posting_link_retirements retirement ON retirement.link_id = link.id
	JOIN import_staged_rows staged ON staged.id = (SELECT MIN(original.id) FROM import_staged_rows original
		WHERE original.committed_identity_id = identity_row.id AND original.commit_status = 'committed')
	JOIN import_batches batch ON batch.id = staged.batch_id
	WHERE link.book_id = ?
		AND link.id = (SELECT MAX(latest.id) FROM import_posting_links latest WHERE latest.identity_id = link.identity_id)`

func scanImportStatementLink(row importStagedRowScanner) (ImportStatementLinkRecord, error) {
	var record ImportStatementLinkRecord
	var active int
	err := row.Scan(&record.IdentityID, &record.LinkID, &record.TransactionID, &record.PostingLineID, &record.LineKey, &record.AccountID,
		&active, &record.LinkedAt, &record.RetiredAt, &record.RetireReason,
		&record.SourceKind, &record.BatchID, &record.Filename, &record.NormalizedJSON, &record.ResolutionJSON)
	record.Active = active != 0
	return record, err
}

// ImportStatementLinksForTransaction lists the statement rows whose latest
// link names one of the transaction's lines: rows linked here now, and rows
// last linked here and since unlinked, so the owner can link them again.
func (r *ImportRepository) ImportStatementLinksForTransaction(ctx context.Context, bookID, transactionID int64) ([]ImportStatementLinkRecord, error) {
	rows, err := r.database.QueryContext(ctx, importStatementLinkSelect+`
		AND link.transaction_id = ? ORDER BY line.id, link.id`, bookID, transactionID)
	if err != nil {
		return nil, fmt.Errorf("read statement links: %w", err)
	}
	defer rows.Close()
	var records []ImportStatementLinkRecord
	for rows.Next() {
		record, err := scanImportStatementLink(rows)
		if err != nil {
			return nil, fmt.Errorf("scan statement link: %w", err)
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// ImportStatementLinkByIdentity reads one statement row's latest link.
func (r *ImportRepository) ImportStatementLinkByIdentity(ctx context.Context, bookID, identityID int64) (ImportStatementLinkRecord, error) {
	record, err := scanImportStatementLink(r.database.QueryRowContext(ctx, importStatementLinkSelect+`
		AND link.identity_id = ?`, bookID, identityID))
	if errors.Is(err, sql.ErrNoRows) {
		return ImportStatementLinkRecord{}, ErrNotFound
	}
	if err != nil {
		return ImportStatementLinkRecord{}, fmt.Errorf("read statement link: %w", err)
	}
	return record, nil
}

// ImportPostingLinkCorrectionParams names the statement row, the link the
// owner reviewed (its latest), the reason, and for a reassignment the
// posting it moves to.
type ImportPostingLinkCorrectionParams struct {
	BookID         int64
	IdentityID     int64
	ExpectedLinkID int64
	// ExpectedLinkActive is the reviewed state of that link: a reassignment
	// prepared while it was active is refused once it has been unlinked, and
	// relinking an unlinked row needs a review of it as unlinked (#485).
	ExpectedLinkActive bool
	Reason             string
	TransactionID      int64
	LineKey            string
	ActorUserID        int64
	AuthSessionID      int64
	RequestID          string
	OccurredAt         string
}

// latestImportPostingLinkTx reads a row's latest link and whether it is
// active, inside the correction's transaction.
func latestImportPostingLinkTx(ctx context.Context, tx *sql.Tx, bookID, identityID int64) (int64, bool, error) {
	var linkID int64
	var active int
	err := tx.QueryRowContext(ctx, `
		SELECT link.id, NOT EXISTS (SELECT 1 FROM import_posting_link_retirements r WHERE r.link_id = link.id)
		FROM import_posting_links link WHERE link.book_id = ? AND link.identity_id = ?
		ORDER BY link.id DESC LIMIT 1`, bookID, identityID).Scan(&linkID, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, ErrNotFound
	}
	if err != nil {
		return 0, false, fmt.Errorf("read latest statement link: %w", err)
	}
	return linkID, active != 0, nil
}

func retireImportPostingLinkTx(ctx context.Context, tx *sql.Tx, params ImportPostingLinkCorrectionParams, linkID, auditEventID int64) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO import_posting_link_retirements (link_id, book_id, reason, retired_audit_event_id, retired_at)
		VALUES (?, ?, ?, ?, ?)`, linkID, params.BookID, params.Reason, auditEventID, params.OccurredAt); err != nil {
		return fmt.Errorf("retire statement link: %w", err)
	}
	return nil
}

// UnlinkImportedPosting retires a statement row's active link (#485 review).
// The link stays as evidence; the row keeps its identity, so re-importing it
// is still a duplicate, and it can be linked again. A link other than the
// one the owner reviewed refuses with ErrImportPostingLinkStale.
func (r *ImportRepository) UnlinkImportedPosting(ctx context.Context, params ImportPostingLinkCorrectionParams) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin unlink statement row: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
	linkID, active, err := latestImportPostingLinkTx(ctx, tx, params.BookID, params.IdentityID)
	if err != nil {
		return err
	}
	if linkID != params.ExpectedLinkID || !active || !params.ExpectedLinkActive {
		return ErrImportPostingLinkStale
	}
	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID: params.BookID, ActorUserID: params.ActorUserID, AuthSessionID: params.AuthSessionID,
		OccurredAt: params.OccurredAt, RequestID: params.RequestID, OriginType: "browser_api",
		Operation: "import.posting.unlink", Reason: params.Reason,
	})
	if err != nil {
		return err
	}
	if err := retireImportPostingLinkTx(ctx, tx, params, linkID, auditEventID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit unlink statement row: %w", err)
	}
	committed = true
	return nil
}

// ReassignImportedPosting moves a statement row's link to another posting
// (#485 review): in one transaction it checks the reviewed link is still the
// latest, rereads the target and lets verify judge it against the row,
// retires the current link (if still active) and appends its successor,
// under one audit event.
func (r *ImportRepository) ReassignImportedPosting(ctx context.Context, params ImportPostingLinkCorrectionParams, verify func(candidate ImportPostingCandidateRecord, found bool) error) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin reassign statement row: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
	linkID, active, err := latestImportPostingLinkTx(ctx, tx, params.BookID, params.IdentityID)
	if err != nil {
		return err
	}
	if linkID != params.ExpectedLinkID || active != params.ExpectedLinkActive {
		return ErrImportPostingLinkStale
	}
	candidate, err := scanImportPostingCandidate(tx.QueryRowContext(ctx, importPostingCandidateSelect+`
			AND t.id = ? AND line.line_key = ?`, params.BookID, params.TransactionID, params.LineKey))
	found := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("reread reassignment posting: %w", err)
	}
	if err := verify(candidate, found); err != nil {
		return err
	}
	if !found || candidate.Linked {
		return ErrImportPostingLinkStale
	}
	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID: params.BookID, ActorUserID: params.ActorUserID, AuthSessionID: params.AuthSessionID,
		OccurredAt: params.OccurredAt, RequestID: params.RequestID, OriginType: "browser_api",
		Operation: "import.posting.reassign", Reason: params.Reason,
	})
	if err != nil {
		return err
	}
	if active {
		if err := retireImportPostingLinkTx(ctx, tx, params, linkID, auditEventID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO import_posting_links (identity_id, book_id, transaction_id, posting_line_id, linked_posting_version_id, replaces_link_id, created_audit_event_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, params.IdentityID, params.BookID, candidate.TransactionID, candidate.PostingLineID, candidate.PostingVersionID, linkID, auditEventID, params.OccurredAt); err != nil {
		return fmt.Errorf("append reassigned statement link: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reassign statement row: %w", err)
	}
	committed = true
	return nil
}
