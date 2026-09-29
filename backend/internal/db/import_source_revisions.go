package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrImportSourceRevisionConflict = errors.New("import source revision is no longer eligible")

type CommitImportSourceRevisionParams struct {
	BookID                int64
	IdentityID            int64
	StagedRowID           int64
	SourceOperationID     int64
	CorrectionOperationID int64
	CreatedAuditEventID   int64
	CreatedAt             string
}

// CommitSourceRevisionInTx is called by an investment correction writer after
// it has installed the inverse/replacement journal and replay. The staged
// result and immutable source-to-correction link must share that transaction.
func (r *ImportRepository) CommitSourceRevisionInTx(ctx context.Context, tx *sql.Tx, params CommitImportSourceRevisionParams) error {
	if params.BookID <= 0 || params.IdentityID <= 0 || params.StagedRowID <= 0 ||
		params.SourceOperationID <= 0 || params.CorrectionOperationID <= 0 ||
		params.CreatedAuditEventID <= 0 || params.CreatedAt == "" {
		return fmt.Errorf("%w: incomplete source revision", ErrImportSourceRevisionConflict)
	}
	var originalTransactionID int64
	var stagedStatus string
	var sourceChanged int
	err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(effect.transaction_id, 0), staged.commit_status,
			EXISTS(SELECT 1 FROM import_staged_rows latest
				WHERE latest.committed_identity_id = identity_row.id
					AND latest.commit_status = 'committed'
					AND latest.id = (SELECT MAX(accepted.id) FROM import_staged_rows accepted
						WHERE accepted.committed_identity_id = identity_row.id AND accepted.commit_status = 'committed')
					AND staged.id > latest.id
					AND (json_remove(latest.raw_json, '$.resolved_commodity_id', '$.resolved_holding_account_id') <>
						json_remove(staged.raw_json, '$.resolved_commodity_id', '$.resolved_holding_account_id')
						OR latest.normalized_json <> staged.normalized_json))
		FROM import_commit_identities identity_row
		JOIN import_commit_identity_effects effect ON effect.identity_id = identity_row.id
		JOIN import_staged_rows staged ON staged.book_id = identity_row.book_id
			AND staged.dedupe_fingerprint = identity_row.dedupe_fingerprint
		JOIN import_batches batch ON batch.id = staged.batch_id AND batch.book_id = identity_row.book_id
		WHERE identity_row.id = ? AND identity_row.book_id = ?
			AND identity_row.source_kind = 'trading212'
			AND batch.source_kind = identity_row.source_kind
			AND batch.status IN ('previewing', 'partially_committed', 'committed', 'failed')
			AND effect.operation_id = ? AND staged.id = ?
			AND json_extract(staged.raw_json, '$.kind') = 'trading212_order_fill'
	`, params.IdentityID, params.BookID, params.SourceOperationID, params.StagedRowID).
		Scan(&originalTransactionID, &stagedStatus, &sourceChanged)
	if errors.Is(err, sql.ErrNoRows) ||
		(stagedStatus != "pending" && stagedStatus != "skipped") ||
		sourceChanged == 0 || originalTransactionID <= 0 {
		return ErrImportSourceRevisionConflict
	}
	if err != nil {
		return fmt.Errorf("check source revision identity: %w", err)
	}
	var descendant int
	err = tx.QueryRowContext(ctx, `WITH RECURSIVE ancestors(id, parent_id) AS (
		SELECT id, correction_of_operation_id FROM investment_operations
		WHERE book_id = ? AND id = ? AND created_audit_event_id = ?
		UNION ALL
		SELECT parent.id, parent.correction_of_operation_id
		FROM investment_operations parent JOIN ancestors ancestor ON parent.id = ancestor.parent_id
		WHERE parent.book_id = ?
	)
	SELECT EXISTS(SELECT 1 FROM ancestors WHERE id = ?)`,
		params.BookID, params.CorrectionOperationID, params.CreatedAuditEventID,
		params.BookID, params.SourceOperationID).Scan(&descendant)
	if err != nil {
		return fmt.Errorf("check source revision correction lineage: %w", err)
	}
	if descendant == 0 {
		return ErrImportSourceRevisionConflict
	}
	if err := r.CommitImportStagedRowInTx(ctx, tx, CommitImportStagedRowParams{
		RowID: params.StagedRowID, CommitStatus: "committed",
		CommittedIdentityID:    sql.NullInt64{Int64: params.IdentityID, Valid: true},
		CommittedTransactionID: sql.NullInt64{Int64: originalTransactionID, Valid: true},
	}); err != nil {
		return fmt.Errorf("mark revised source row committed: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO import_source_revisions
		(book_id, identity_id, staged_row_id, source_operation_id,
		 correction_operation_id, created_audit_event_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, params.BookID, params.IdentityID,
		params.StagedRowID, params.SourceOperationID,
		params.CorrectionOperationID, params.CreatedAuditEventID, params.CreatedAt); err != nil {
		return fmt.Errorf("record import source revision: %w", err)
	}
	return nil
}
