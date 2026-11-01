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
	SELECT t.id, tv.id, line.id, line.line_key, tv.transaction_date, tv.transaction_kind,
		entry.entry_date, pv.account_id, pv.commodity_id, pv.quantity_value, pv.quantity_scale,
		COALESCE(tv.payee_name, ''), tv.description,
		(EXISTS (SELECT 1 FROM import_posting_links link WHERE link.posting_line_id = line.id)
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
	if err := row.Scan(&record.TransactionID, &record.TransactionVersionID, &record.PostingLineID, &record.LineKey,
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
		INSERT INTO import_posting_links (identity_id, book_id, transaction_id, posting_line_id, created_audit_event_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, identityID, identity.BookID, candidate.TransactionID, candidate.PostingLineID, auditEventID, identity.CreatedAt); err != nil {
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

// ImportPostingLinkRecord is the posting a committed statement row links to.
type ImportPostingLinkRecord struct {
	IdentityID    int64
	TransactionID int64
	PostingLineID int64
	LineKey       string
}

// ImportPostingLinksForTransaction lists the statement-row links to a
// transaction's lines, in line order.
func (r *ImportRepository) ImportPostingLinksForTransaction(ctx context.Context, bookID, transactionID int64) ([]ImportPostingLinkRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT link.identity_id, link.transaction_id, link.posting_line_id, line.line_key
		FROM import_posting_links link JOIN posting_lines line ON line.id = link.posting_line_id
		WHERE link.book_id = ? AND link.transaction_id = ? ORDER BY line.id`, bookID, transactionID)
	if err != nil {
		return nil, fmt.Errorf("read import posting links: %w", err)
	}
	defer rows.Close()
	var records []ImportPostingLinkRecord
	for rows.Next() {
		var record ImportPostingLinkRecord
		if err := rows.Scan(&record.IdentityID, &record.TransactionID, &record.PostingLineID, &record.LineKey); err != nil {
			return nil, fmt.Errorf("scan import posting link: %w", err)
		}
		records = append(records, record)
	}
	return records, rows.Err()
}
